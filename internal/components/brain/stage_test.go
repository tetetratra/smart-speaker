package brain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/features/memory"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/tools/execution"
	"github.com/tetetratra/smart-speaker/internal/types"
)

type lightTool struct{ calls []map[string]any }

func (l *lightTool) Name() string { return "switchbot_execute_scene" }
func (l *lightTool) Run(_ context.Context, args map[string]any) (map[string]any, error) {
	l.calls = append(l.calls, args)
	return map[string]any{"ok": true}, nil
}

type fakeAPI struct {
	mu       sync.Mutex
	payloads []map[string]any
	replies  []string
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	f.payloads = append(f.payloads, payload)
	reply := f.replies[0]
	f.replies = f.replies[1:]
	_, _ = w.Write([]byte(reply))
}

func testStage(t *testing.T, api *fakeAPI, tool tools.Handler) *stage {
	t.Helper()
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	history, err := conversationhistory.NewStore(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := history.Append(types.ConversationRecord{ID: "1", Role: types.RoleUser, Text: "電気消して", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	memories, err := memory.NewStore(filepath.Join(t.TempDir(), "memories.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := memories.Upsert(memory.UpsertInput{Content: "寝室の照明はシーン名『消灯』"}); err != nil {
		t.Fatal(err)
	}
	gen := generation.NewStore()
	gen.Next()
	return &stage{node: &graph.Stage{Downstream: make(chan types.Event, 20)}, cfg: Config{APIKey: "local-test", Endpoint: server.URL, Client: server.Client(),
		Model: "brain-model", ReasoningEffort: "low",
		Instructions: "頭脳AIの指示", Tools: []any{map[string]any{"type": "function", "name": "switchbot_execute_scene"}},
		Executor: execution.New(map[string]tools.Handler{"switchbot_execute_scene": tool}), History: history, Memory: memories, Generation: gen}}
}

func records(s *stage) []types.ConversationInput {
	var got []types.ConversationInput
	for len(s.node.Downstream) > 0 {
		got = append(got, (<-s.node.Downstream).(types.EventConversationInput).ConversationInput)
	}
	return got
}

// ツールを呼んで結果を次の呼び出しに戻し、最後に話す内容を記録として返す。ツールの記録は画面に出す。
func TestThinkCallsToolsAndReturnsAnswer(t *testing.T) {
	api := &fakeAPI{replies: []string{
		`{"output":[{"type":"reasoning","id":"rs_1","encrypted_content":"x"},{"type":"function_call","call_id":"call1","name":"switchbot_execute_scene","arguments":"{\"scene\":\"消灯\"}"}]}`,
		`{"output":[{"type":"message","content":[{"type":"output_text","text":"寝室の電気を消したよ。"}]}]}`,
	}}
	tool := &lightTool{}
	s := testStage(t, api, tool)
	s.think(context.Background(), types.BrainRequest{RequestID: "req1", GenerationID: s.cfg.Generation.Current()})
	if len(tool.calls) != 1 || tool.calls[0]["scene"] != "消灯" {
		t.Fatal(tool.calls)
	}
	got := records(s)
	if len(got) != 2 {
		t.Fatal(got)
	}
	if call := got[0]; call.Record.Role != types.RoleTool || call.Record.CallID != "call1" || call.UserAlreadyKnows || !call.ModelAlreadyKnows {
		t.Fatal(call)
	}
	answer := got[1]
	if answer.Record.Source != types.BrainSource || answer.Record.Text != "寝室の電気を消したよ。" || answer.Record.Metadata[types.BrainRequestMetadataKey] != "req1" || !answer.UserAlreadyKnows {
		t.Fatal(answer)
	}
	first := api.payloads[0]
	if first["model"] != "brain-model" || first["reasoning"].(map[string]any)["effort"] != "low" || first["store"] != false {
		t.Fatal(first)
	}
	prompt := first["input"].([]any)[0].(map[string]any)["content"].(string)
	if !strings.Contains(prompt, "電気消して") || !strings.Contains(prompt, "寝室の照明はシーン名『消灯』") {
		t.Fatal("履歴と長期メモリを渡していません", prompt)
	}
	// 毎回変わる日時は、キャッシュを崩さないよう指示ではなく入力の末尾に置く。
	if first["instructions"] != "頭脳AIの指示" || !strings.Contains(prompt[strings.Index(prompt, "## 依頼"):], "## 現在の日時") {
		t.Fatal("日時の置き場所が違います", first["instructions"])
	}
	var names []string
	for _, tool := range first["tools"].([]any) {
		m := tool.(map[string]any)
		names = append(names, m["type"].(string)+":"+func() string { s, _ := m["name"].(string); return s }())
	}
	if strings.Join(names, ",") != "function:switchbot_execute_scene,web_search:" {
		t.Fatal("組み込みの web_search を渡していません", names)
	}
	// 2回目は、1回目の出力（推論・関数呼び出し）と関数の結果を入力に足す。
	second := api.payloads[1]["input"].([]any)
	if len(second) != 4 || second[3].(map[string]any)["type"] != "function_call_output" || second[3].(map[string]any)["call_id"] != "call1" {
		t.Fatal(second)
	}
}

// 何も出力しなければ、話さないことを印付きで記録する。失敗したら、そのことを伝える内容を返す。
func TestThinkReportsSilenceAndFailure(t *testing.T) {
	api := &fakeAPI{replies: []string{`{"output":[]}`, `broken`}}
	s := testStage(t, api, &lightTool{})
	s.think(context.Background(), types.BrainRequest{RequestID: "silent"})
	silent := records(s)[0]
	if silent.Record.Metadata[types.BrainSilentMetadataKey] != true || api.payloads[0]["model"] != "brain-model" {
		t.Fatal(silent)
	}
	s.think(context.Background(), types.BrainRequest{RequestID: "failed"})
	failed := records(s)[0]
	if failed.Record.Text != failedText || failed.Record.Metadata[types.BrainSilentMetadataKey] != nil {
		t.Fatal(failed)
	}
}

// ユーザーが話し足して世代が進んだら、次の API 呼び出しをせずにやめる。呼んだツールの記録は残す。
func TestThinkStopsWhenGenerationAdvances(t *testing.T) {
	api := &fakeAPI{replies: []string{
		`{"output":[{"type":"function_call","call_id":"call1","name":"switchbot_execute_scene","arguments":"{}"}]}`,
	}}
	tool := &lightTool{}
	s := testStage(t, api, tool)
	gen := s.cfg.Generation.Current()
	s.cfg.Executor = execution.New(map[string]tools.Handler{"switchbot_execute_scene": advancing{tool: tool, gen: s.cfg.Generation}})
	s.think(context.Background(), types.BrainRequest{RequestID: "req1", GenerationID: gen})
	got := records(s)
	if len(api.payloads) != 1 || len(got) != 1 || got[0].Record.Role != types.RoleTool {
		t.Fatal(len(api.payloads), got)
	}
}

// advancing は、実行中にユーザーが話し始めて世代が進んだことにするツール。
type advancing struct {
	tool *lightTool
	gen  *generation.Store
}

func (a advancing) Name() string { return a.tool.Name() }
func (a advancing) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	a.gen.Next()
	return a.tool.Run(ctx, args)
}

// 話す内容から、組み込みの web_search が混ぜる引用の印・出典のリンク・Markdown の強調を取り除く。
func TestSpeakableRemovesCitationsLinksAndEmphasis(t *testing.T) {
	for in, want := range map[string]string{
		"本セールは、**12月7日（月）23時59分まで**。([example.com](https://example.com/a?utm_source=openai))": "本セールは、12月7日（月）23時59分まで。",
		"在庫は変わるよ。 \ue200cite\ue202turn0search0\ue202turn1search0\ue201":                       "在庫は変わるよ。",
		"詳しくは[公式サイト](https://example.com)を見てね。https://example.com/b":                          "詳しくは公式サイトを見てね。",
	} {
		if got := speakable(in); got != want {
			t.Fatalf("%q => %q", in, got)
		}
	}
}
