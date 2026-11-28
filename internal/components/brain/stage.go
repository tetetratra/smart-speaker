// Package brain は頭脳AI。会話の履歴と長期メモリを読み、ツールを呼びながら、ユーザーに話す内容を考える。
// 依頼（EventBrainRequest）ごとに Responses API を呼び、ツールの記録と話す内容を記録（EventConversationInput）として返す。
// 話す内容をどう伝えるかは会話の流れ（conversationflow）と realtime-response が決める。
package brain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tetetratra/smart-speaker/internal/conversation"
	"github.com/tetetratra/smart-speaker/internal/features/memory"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/tools/execution"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// ModelConfig は、軽いモデル・強いモデルそれぞれの設定。
type ModelConfig struct {
	Model string
	// ReasoningEffort は Responses API の reasoning.effort（none・low・medium など）。空なら API の既定値。
	ReasoningEffort string
}

type Config struct {
	APIKey   string
	Endpoint string
	Light    ModelConfig
	Strong   ModelConfig
	// Instructions は頭脳AIへの指示（システムプロンプトとプラグインの指示）。
	Instructions string
	// Tools は function calling 用のツール定義。組み込みの web_search はここに含めず、頭脳AIが足す。
	Tools      []any
	Executor   *execution.Executor
	History    *conversationhistory.Store
	Memory     *memory.Store
	Generation *generation.Store
	// Client はテストで差し替える。nil なら時間切れを付けたものを使う。
	Client *http.Client
}

// maxSteps は、1回の依頼で Responses API を呼ぶ回数の上限。ツールを呼び続けて終わらないのを防ぐ。
const maxSteps = 10

type stage struct {
	node  *graph.Stage
	cfg   Config
	tasks sync.WaitGroup
}

func NewStage(cfg Config) (*graph.Stage, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("brain: API key is required")
	}
	if cfg.History == nil || cfg.Generation == nil || cfg.Executor == nil {
		return nil, fmt.Errorf("brain: 履歴・世代のStoreとツールの実行器が必要")
	}
	if cfg.Light.Model == "" || cfg.Strong.Model == "" {
		return nil, fmt.Errorf("brain: 軽いモデルと強いモデルを指定してください")
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://api.openai.com/v1/responses"
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 90 * time.Second}
	}
	s := &stage{cfg: cfg}
	s.node = &graph.Stage{Name: "brain", Upstream: make(chan types.Event), Downstream: make(chan types.Event), Run: s.run, CloseFn: func() error { return nil }}
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
		req, ok := e.(types.EventBrainRequest)
		if !ok {
			continue
		}
		// 考えている間も、後から届いた記録への対応など別の依頼を並行して受ける。
		s.tasks.Add(1)
		go func() {
			defer s.tasks.Done()
			s.think(ctx, req.BrainRequest)
		}()
	}
}

func (s *stage) emit(ctx context.Context, record types.ConversationRecord, userAlreadyKnows bool) {
	input := types.ConversationInput{Record: record, ModelAlreadyKnows: true, UserAlreadyKnows: userAlreadyKnows}
	select {
	case s.node.Downstream <- types.EventConversationInput{ConversationInput: input}:
	case <-ctx.Done():
	}
}

// stale は、ユーザーの発話から任された依頼で、ユーザーが話し足して世代が進んだかを返す。
func (s *stage) stale(req types.BrainRequest) bool {
	return req.GenerationID != 0 && !s.cfg.Generation.IsCurrent(req.GenerationID)
}

// think は依頼1件を考え、話す内容を記録として返す。
// ツールの副作用は取り消せないので、呼んだツールは記録に残す。世代が進んだら、次の API 呼び出しをせずにやめる。
func (s *stage) think(ctx context.Context, req types.BrainRequest) {
	model := s.cfg.Strong
	if req.Model == types.BrainModelLight {
		model = s.cfg.Light
	}
	started := time.Now()
	// 何に対応する依頼かを後から追えるよう、依頼の時点で履歴の最後にある記録（ユーザーの発話や後から届いた結果）をログに残す。
	if history := s.cfg.History.Snapshot(); len(history) > 0 {
		last := history[len(history)-1]
		log.Printf("brain: 依頼 %s 受付 model=%s generation=%d 対象=%s(%s) %q", req.RequestID, model.Model, req.GenerationID, last.Role, last.Source, last.Text)
	}
	input := []json.RawMessage{s.prompt()}
	for step := 0; step < maxSteps; step++ {
		if s.stale(req) {
			log.Printf("brain: 依頼 %s はユーザーが話し足したため打ち切りました", req.RequestID)
			return
		}
		out, err := s.call(ctx, model, input)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("brain: 依頼 %s の呼び出しに失敗しました: %v", req.RequestID, err)
			s.say(ctx, req, failedText, false)
			return
		}
		input = append(input, out.items...)
		if len(out.calls) == 0 {
			log.Printf("brain: 依頼 %s 完了 model=%s 回数=%d 所要=%s", req.RequestID, model.Model, step+1, time.Since(started).Round(time.Millisecond))
			text := strings.TrimSpace(out.text)
			s.say(ctx, req, text, text == "")
			return
		}
		for _, c := range out.calls {
			result := s.cfg.Executor.Execute(ctx, types.ToolRequest{ToolCallID: c.CallID, Name: c.Name, Arguments: json.RawMessage(c.Arguments), GenerationID: req.GenerationID})
			output := string(result.Output)
			log.Printf("brain: 依頼 %s ツール %s 引数=%s 結果=%s", req.RequestID, c.Name, c.Arguments, output)
			// ツールの実行はユーザーがまだ知らないので、画面に出す。
			s.emit(ctx, types.ConversationRecord{ID: "brain/" + req.RequestID + "/tool/" + c.CallID, Role: types.RoleTool, Text: c.Name + ": " + c.Arguments + " → " + output,
				GenerationID: req.GenerationID, Source: c.Name, CreatedAt: time.Now(), Title: s.cfg.Executor.Title(c.Name, json.RawMessage(c.Arguments)),
				CallID: c.CallID, Arguments: c.Arguments, Output: output, Error: types.OutputError(output)}, false)
			item, _ := json.Marshal(map[string]any{"type": "function_call_output", "call_id": c.CallID, "output": output})
			input = append(input, item)
		}
	}
	log.Printf("brain: 依頼 %s はツール呼び出しが上限の %d 回に達しました", req.RequestID, maxSteps)
	s.say(ctx, req, failedText, false)
}

// failedText は、頭脳AIが答えを用意できなかったときの話す内容。
const failedText = "エラーのため答えを用意できませんでした。そのことを短く伝えてください。"

// silentText は、頭脳AIが何も話さないと判断したときの記録の本文。
const silentText = "[頭脳AIは何も話さないと判断しました]"

// say は話す内容を記録として返す。silent なら何も話さないことを記録し、会話の流れには応答させない印を付ける。
// 話す内容は realtime-response が話し言葉にして伝え、その発話が画面に出るので、この記録は画面に出さない。
func (s *stage) say(ctx context.Context, req types.BrainRequest, text string, silent bool) {
	metadata := map[string]any{types.BrainRequestMetadataKey: req.RequestID}
	if silent {
		text = silentText
		metadata[types.BrainSilentMetadataKey] = true
	} else {
		text = speakable(text)
	}
	log.Printf("brain: 依頼 %s 応答 %q", req.RequestID, text)
	s.emit(ctx, types.ConversationRecord{ID: "brain/" + req.RequestID + "/" + fmt.Sprint(time.Now().UnixNano()), Role: types.RoleBackground, Text: text,
		GenerationID: req.GenerationID, Source: types.BrainSource, Metadata: metadata, CreatedAt: time.Now()}, true)
}

// prompt は会話の履歴と長期メモリを1件の入力にする。先頭がそろうよう、変わりにくい長期メモリを先に、増えていく会話を後に置く。
func (s *stage) prompt() json.RawMessage {
	var memories []string
	if s.cfg.Memory != nil {
		for _, record := range s.cfg.Memory.Snapshot() {
			if content := strings.TrimSpace(record.Content); content != "" {
				memories = append(memories, content)
			}
		}
	}
	// 現在の日時は毎回変わるので、指示ではなく入力の末尾に置く。指示に入れると、その後ろに続く会話の履歴がキャッシュに当たらなくなるため。
	text := conversation.RestoreContext(s.cfg.History.Snapshot(), memories) + "\n" + requestInstructions + "\n\n" + strings.TrimSpace(conversation.WithCurrentTime("", time.Now()))
	raw, _ := json.Marshal(map[string]any{"role": "user", "content": text})
	return raw
}

const requestInstructions = `## 依頼
上の会話の最後の記録（ユーザーの発話、または後から届いた結果や知らせ）に対応してください。
過去の会話の記録にある操作は、新しい依頼として再実行しないでください。`

var (
	// citationMarks は、組み込みの web_search が本文に埋め込む引用の印（私用領域の文字で囲まれた turn0search0 など）。
	citationMarks = regexp.MustCompile(`\x{e200}[^\x{e201}]*\x{e201}`)
	// parenLinks は「([prtimes.jp](https://...))」のような、出典を示す括弧書きのリンク。
	parenLinks = regexp.MustCompile(`\s*[（(]\s*\[[^\]]*\]\([^)]*\)\s*[)）]`)
	// links は「[文字](URL)」の形のリンク。文字だけを残す。
	links = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	// bareURLs は、文中にそのまま書かれた URL。
	bareURLs = regexp.MustCompile(`https?://\S+`)
)

// speakable は、話す内容から、読み上げられない引用の印・リンク・Markdown の強調を取り除く。
// 組み込みの web_search の結果をもとにすると、プロンプトで頼んでもこれらが混ざるため。
func speakable(text string) string {
	text = citationMarks.ReplaceAllString(text, "")
	text = parenLinks.ReplaceAllString(text, "")
	text = links.ReplaceAllString(text, "$1")
	text = bareURLs.ReplaceAllString(text, "")
	text = strings.NewReplacer("**", "", "__", "").Replace(text)
	return strings.TrimSpace(text)
}

type functionCall struct {
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type output struct {
	// items は、次の呼び出しの入力へそのまま戻す出力（推論・関数呼び出し・メッセージなど）。
	items []json.RawMessage
	calls []functionCall
	text  string
}

func (s *stage) call(ctx context.Context, model ModelConfig, input []json.RawMessage) (output, error) {
	payload := map[string]any{
		"model":        model.Model,
		"instructions": s.cfg.Instructions,
		"input":        input,
		"tools":        append(append([]any{}, s.cfg.Tools...), map[string]any{"type": "web_search"}),
		// 会話の状態は API 側に持たせず、毎回すべてを渡す。推論の中身は暗号化したものを受け取って次の呼び出しに戻す。
		"store":   false,
		"include": []string{"reasoning.encrypted_content"},
	}
	if model.ReasoningEffort != "" {
		payload["reasoning"] = map[string]any{"effort": model.ReasoningEffort}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return output{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return output{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	resp, err := s.cfg.Client.Do(httpReq)
	if err != nil {
		return output{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return output{}, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var decoded struct {
		Output []json.RawMessage `json:"output"`
		Usage  struct {
			InputTokens        int `json:"input_tokens"`
			InputTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return output{}, fmt.Errorf("応答の読み取り: %w", err)
	}
	log.Printf("brain: 使用量 model=%s 入力=%d（キャッシュ=%d） 出力=%d", model.Model, decoded.Usage.InputTokens, decoded.Usage.InputTokensDetails.CachedTokens, decoded.Usage.OutputTokens)
	var out output
	var text strings.Builder
	for _, raw := range decoded.Output {
		var item struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			functionCall
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return output{}, fmt.Errorf("出力の読み取り: %w", err)
		}
		out.items = append(out.items, raw)
		switch item.Type {
		case "function_call":
			out.calls = append(out.calls, item.functionCall)
		case "message":
			for _, part := range item.Content {
				if part.Type == "output_text" {
					text.WriteString(part.Text)
				}
			}
		}
	}
	out.text = text.String()
	return out, nil
}
