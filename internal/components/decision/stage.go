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
	// previous は前回の判定に渡した会話。キャッシュの境目を置く位置を決めるのに使う。判定は並行して動くので mu で守る。
	mu       sync.Mutex
	previous string
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
		// 判定が返らないとユーザーは無言で待つことになるので、長く待たずに頭脳AIへ回す。
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
			choice := s.decide(ctx)
			select {
			case s.node.Downstream <- types.EventDecision{Decision: types.Decision{TurnID: req.TurnID, GenerationID: req.GenerationID, Choice: choice}}:
			case <-ctx.Done():
			}
		}()
	}
}

// decide は判定する。API が失敗したときや答えを拒否したときは、頭脳AIに任せる。
// 頭脳AIなら自分宛てでない発話にも「何も話さない」と答えられるので、無言のまま止まるより安全なため。
func (s *stage) decide(ctx context.Context) types.DecisionChoice {
	started := time.Now()
	answer, err := s.call(ctx, s.input())
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("decision: 判定に失敗したため頭脳AIに任せます: %v", err)
		}
		return types.DecisionDelegate
	}
	choice := types.DecisionChoice(answer.choice)
	switch choice {
	case types.DecisionRespond, types.DecisionDelegate, types.DecisionWait, types.DecisionIgnore:
	default:
		log.Printf("decision: 想定外の判定 %q のため頭脳AIに任せます", answer.choice)
		choice = types.DecisionDelegate
	}
	// 判定に迷っているときは頭脳AIに任せる。頭脳AIなら、そのまま返す・待つ・何も話さない、も選べるため。
	if answer.confidence < lowConfidence && choice != types.DecisionDelegate {
		log.Printf("decision: 判定 %s は confidence が低いため頭脳AIに任せます", choice)
		choice = types.DecisionDelegate
	}
	// 振り分けの精度と待ち時間を後から確かめられるよう、確率・所要時間・キャッシュの効き具合を残す。
	log.Printf("decision: 判定=%s confidence=%.2f 確率=%s 所要=%s 入力=%d（キャッシュ=%d、書き込み=%d）", choice, answer.confidence, answer.probabilities, time.Since(started).Round(time.Millisecond), answer.inputTokens, answer.cachedTokens, answer.cacheWriteTokens)
	return choice
}

// lowConfidence は、判定に迷っているとみなす confidence。これより低ければ頭脳AIに任せる。値は本番ログを見て決めた目安。
const lowConfidence = 0.5

// currentSession は、履歴のうち、Realtime の今のセッション（最後のセッション開始の目印より後）の記録だけを返す。
// 振り分けで知りたいのは「Realtime が今持っている情報で答えられるか」なので、Realtime と同じ範囲だけを渡す。
// 目印がまだなければ（このしくみを入れる前の履歴）、すべてを返す。
func currentSession(records []types.ConversationRecord) []types.ConversationRecord {
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Source == types.SessionStartSource {
			return records[i+1:]
		}
	}
	return records
}

// input は Decisions API に渡す会話を、キャッシュの境目で区切った部分に分けて返す。
// 今回の発話は記録係が保存済みなので、履歴の最後のユーザーの行に入っている。
//
// Decisions API の既定のキャッシュは境目が入力の末尾に1つだけで、入力全体が前回と同じときしか当たらない。
// 会話の履歴は末尾に増えていくので、境目を明示し、前回の入力の末尾と今回の入力の末尾の2か所に置く。
// 前回の末尾の境目で前回書き込んだ分に当たり、今回の末尾の境目で次回のために書き込む（実 API で確認済み）。
func (s *stage) input() []string {
	full := conversation.RestoreContext(currentSession(s.cfg.History.Snapshot()), nil)
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.previous
	s.previous = full
	// 古い履歴がメモリ化されて先頭が変わったときは、前回の分には当たらないので、末尾にだけ境目を置く。
	if previous != "" && len(full) > len(previous) && strings.HasPrefix(full, previous) {
		return []string{previous, full[len(previous):]}
	}
	return []string{full}
}

const questionInstructions = `あなたは家庭用の音声アシスタントの振り分け役です。音声で話すAIはツールを持たず、渡した会話（今の会話セッションのやりとり）しか知りません。
会話の中でいちばん新しいユーザー（user）の発話に、音声で話すAIがそのまま答えられるか、裏の頭脳AIに任せるべきかなどを選んでください。
会話には、頭脳AIが前に用意した内容（background(brain) の行。「話すこと」と「参考」）も含まれます。音声で話すAIはこれも知っています。
音声は自動で文字起こししているため、聞き取りの誤りや、物音・テレビ・ほかの人の声が混ざることがあります。`

var choices = []map[string]any{
	{"value": string(types.DecisionRespond), "description": "挨拶・お礼・雑談・呼びかけだけ、直前のやりとりの続き（「他には？」「それって？」「じゃあそれで」）など、新しい操作や調べものが要らず、渡した会話（頭脳AIが前に用意した内容も含む）の範囲で答えられる発話。"},
	{"value": string(types.DecisionDelegate), "description": "家電の操作、予定・天気・予約ジョブ・メモリなどアプリが持つ情報の確認、調べもの、判断や提案が要る依頼。渡した会話にない、以前の話や覚えていることを尋ねる質問も含む。迷う場合もこれを選ぶ。"},
	{"value": string(types.DecisionWait), "description": "「えっと」「うーん」のように話し始めただけで、自分への依頼の続きがありそうな発話。"},
	{"value": string(types.DecisionIgnore), "description": "物音、テレビやほかの人の声、独り言、意味の通らない断片など、自分への依頼ではない入力。アシスタントの答えを受けた相槌（「うん」「そうそう」「なるほど」）で、返事が要らない発話も含む。ユーザーが待つよう明示した場合も含む。"},
}

type answer struct {
	choice                                      string
	confidence                                  float64
	probabilities                               string
	inputTokens, cachedTokens, cacheWriteTokens int
}

func (s *stage) call(ctx context.Context, parts []string) (answer, error) {
	content := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		content = append(content, map[string]any{"type": "input_text", "text": part, "prompt_cache_breakpoint": map[string]any{"mode": "explicit"}})
	}
	payload := map[string]any{
		"model":                s.cfg.Model,
		"input":                []map[string]any{{"role": "user", "content": content}},
		"prompt_cache_options": map[string]any{"mode": "explicit", "ttl": "30m"},
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
				CachedTokens     int `json:"cached_tokens"`
				CacheWriteTokens int `json:"cache_write_tokens"`
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
	return answer{choice: choice, confidence: a.Confidence, probabilities: string(a.Probabilities), inputTokens: decoded.Usage.InputTokens, cachedTokens: decoded.Usage.InputTokensDetails.CachedTokens, cacheWriteTokens: decoded.Usage.InputTokensDetails.CacheWriteTokens}, nil
}
