// Package decision は、ユーザーの発話1回をどう扱うかを Decisions API で判定する。
// 判定の依頼（EventDecisionRequest）を受け、会話の履歴を共有のストアから読んで判定し、結果（EventDecision）を返す。
// 判定結果をどう使うかは会話の流れ（conversationflow）が決める。
package decision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tetetratra/smart-speaker/internal/conversation"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/types"
)

type Config struct {
	APIKey string
	// Model は Decisions API のモデル。今使えるのは gpt-6-luna だけ。
	Model    string
	Endpoint string
	History  *conversationhistory.Store
	// Client はテストで差し替える。nil なら http.DefaultClient に時間切れを付けたものを使う。
	Client *http.Client
}

type stage struct {
	node  *graph.Stage
	cfg   Config
	tasks sync.WaitGroup
}

func NewStage(cfg Config) (*graph.Stage, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("decision: API key is required")
	}
	if cfg.History == nil {
		return nil, fmt.Errorf("decision: 履歴のStoreが必要")
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-6-luna"
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://api.openai.com/v1/decisions"
	}
	if cfg.Client == nil {
		// 判定が返らないとユーザーは無言で待つことになるので、長く待たずに強いモデルへ回す。
		cfg.Client = &http.Client{Timeout: 10 * time.Second}
	}
	s := &stage{cfg: cfg}
	s.node = &graph.Stage{Name: "decision", Upstream: make(chan types.Event), Downstream: make(chan types.Event), Run: s.run, CloseFn: func() error { return nil }}
	return s.node, nil
}

func (s *stage) run(ctx context.Context) {
	defer close(s.node.Downstream)
	defer s.tasks.Wait()
	for {
		e, ok := s.node.Receive(ctx)
		if !ok {
			return
		}
		req, ok := e.(types.EventDecisionRequest)
		if !ok {
			continue
		}
		// 判定を待つ間も次の依頼を受け取れるよう、API の呼び出しは並行して行う。古い依頼の結果は会話の流れが捨てる。
		s.tasks.Add(1)
		go func() {
			defer s.tasks.Done()
			choice := s.decide(ctx, req.DecisionRequest)
			select {
			case s.node.Downstream <- types.EventDecision{Decision: types.Decision{TurnID: req.TurnID, GenerationID: req.GenerationID, Choice: choice}}:
			case <-ctx.Done():
			}
		}()
	}
}

// decide は判定する。API が失敗したときや答えを拒否したときは、強いモデルに任せる。
// 頭脳AIなら自分宛てでない発話にも「何も話さない」と答えられるので、無言のまま止まるより安全なため。
func (s *stage) decide(ctx context.Context, req types.DecisionRequest) types.DecisionChoice {
	started := time.Now()
	answer, err := s.call(ctx, s.input(req))
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("decision: 判定に失敗したため強いモデルに任せます: %v", err)
		}
		return types.DecisionDelegateStrong
	}
	choice := types.DecisionChoice(answer.choice)
	switch choice {
	case types.DecisionRespond, types.DecisionDelegateLight, types.DecisionDelegateStrong, types.DecisionWait, types.DecisionIgnore:
	default:
		log.Printf("decision: 想定外の判定 %q のため強いモデルに任せます", answer.choice)
		choice = types.DecisionDelegateStrong
	}
	// 振り分けの精度と待ち時間を後から確かめられるよう、確率・所要時間・キャッシュの効き具合を残す。
	log.Printf("decision: 判定=%s confidence=%.2f 確率=%s 所要=%s 入力=%d（キャッシュ=%d）", choice, answer.confidence, answer.probabilities, time.Since(started).Round(time.Millisecond), answer.inputTokens, answer.cachedTokens)
	return choice
}

// input は Decisions API に渡す会話。先頭が毎回そろうよう、履歴を古い順に並べ、今回の発話を最後に置く。
func (s *stage) input(req types.DecisionRequest) string {
	return conversation.RestoreContext(s.cfg.History.Snapshot(), nil) + "\n## 今回のユーザーの発話\n" + req.Text
}

const questionInstructions = `あなたは家庭用の音声アシスタントの振り分け役です。会話の流れを踏まえ、「今回のユーザーの発話」にアシスタントがどう対応するかを選んでください。
音声は自動で文字起こししているため、聞き取りの誤りや、物音・テレビ・ほかの人の声が混ざることがあります。`

var choices = []map[string]any{
	{"value": string(types.DecisionRespond), "description": "挨拶・お礼・相槌・雑談・呼びかけだけなど、ツールも調べものも要らず、会話だけで返事が完結する発話。会話の中にすでに答えがある質問も含む。"},
	{"value": string(types.DecisionDelegateLight), "description": "家電の操作、予定や天気などの取得、メモリの保存など、定型の操作をするか、取得した事実を伝えれば完了する依頼。"},
	{"value": string(types.DecisionDelegateStrong), "description": "複数の情報を照らし合わせて判断・提案する、手順を考える、長い説明をするなど、考える必要がある依頼。軽い依頼か迷う場合もこれを選ぶ。"},
	{"value": string(types.DecisionWait), "description": "「えっと」「うーん」のように話し始めただけで、自分への依頼の続きがありそうな発話。"},
	{"value": string(types.DecisionIgnore), "description": "物音、テレビやほかの人の声、独り言、意味の通らない断片など、自分への依頼ではない入力。ユーザーが待つよう明示した場合も含む。"},
}

type answer struct {
	choice                    string
	confidence                float64
	probabilities             string
	inputTokens, cachedTokens int
}

func (s *stage) call(ctx context.Context, input string) (answer, error) {
	payload := map[string]any{
		"model": s.cfg.Model,
		"input": input,
		"questions": []map[string]any{{
			"type": "choice", "name": "turn", "instructions": questionInstructions, "choices": choices,
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return answer{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return answer{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	resp, err := s.cfg.Client.Do(httpReq)
	if err != nil {
		return answer{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return answer{}, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var decoded struct {
		Answers []struct {
			Type          string          `json:"type"`
			Choice        json.RawMessage `json:"choice"`
			Confidence    float64         `json:"confidence"`
			Probabilities json.RawMessage `json:"probabilities"`
		} `json:"answers"`
		Usage struct {
			InputTokens        int `json:"input_tokens"`
			InputTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return answer{}, fmt.Errorf("応答の読み取り: %w", err)
	}
	if len(decoded.Answers) == 0 || decoded.Answers[0].Type != "choice" {
		return answer{}, fmt.Errorf("判定が返りませんでした（拒否を含む）")
	}
	a := decoded.Answers[0]
	var choice string
	if err := json.Unmarshal(a.Choice, &choice); err != nil {
		return answer{}, fmt.Errorf("判定の読み取り: %w", err)
	}
	return answer{choice: choice, confidence: a.Confidence, probabilities: string(a.Probabilities), inputTokens: decoded.Usage.InputTokens, cachedTokens: decoded.Usage.InputTokensDetails.CachedTokens}, nil
}
