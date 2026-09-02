package llm

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/agentstatus"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	timerstate "github.com/tetetratra/smart-speaker/internal/states/timer"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type fakeClient struct {
	calls     int
	responses []string
	prompts   []string
	messages  [][]types.ChatMessage
}

func (f *fakeClient) CreateResponse(ctx context.Context, messages []types.ChatMessage, systemContent string) (string, error) {
	f.messages = append(f.messages, append([]types.ChatMessage(nil), messages...))
	f.prompts = append(f.prompts, systemContent)
	idx := f.calls
	f.calls++
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	return f.responses[idx], nil
}

type fakeAgentStatus struct {
	status agentstatus.Status
}

func (f fakeAgentStatus) Status() agentstatus.Status { return f.status }

func TestStageAddsIdleFollowupInstructionWhenAgentIsIdle(t *testing.T) {
	client := &fakeClient{responses: []string{`{"items":[]}`}}
	st := &stage{
		client:       client,
		agentStatus:  fakeAgentStatus{status: agentstatus.StatusIdle},
		systemPrompt: buildSystemPrompt("", nil),
	}

	items, err := st.requestTimeline(context.Background(), types.LLMRequest{
		RequestID:    "current",
		Role:         types.RoleUser,
		Text:         "あー",
		GenerationID: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("items len = %d, want 0", len(items))
	}
	if client.calls != 1 {
		t.Fatalf("calls = %d, want 1", client.calls)
	}
	prompt := client.prompts[0]
	for _, want := range []string{
		"長期間無音だった",
		"独り言",
		`{"items":[]}`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want it to contain %q", prompt, want)
		}
	}
}

func TestStageKeepsIdleFollowupInstructionOnRetry(t *testing.T) {
	client := &fakeClient{responses: []string{
		`{"items":[{"type":"speech","text":""}]}`,
		`{"items":[]}`,
	}}
	st := &stage{
		client:       client,
		agentStatus:  fakeAgentStatus{status: agentstatus.StatusIdle},
		systemPrompt: buildSystemPrompt("", nil),
	}

	items, err := st.requestTimeline(context.Background(), types.LLMRequest{
		RequestID:    "current",
		Role:         types.RoleUser,
		Text:         "あー",
		GenerationID: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("items len = %d, want 0", len(items))
	}
	if client.calls != 2 {
		t.Fatalf("calls = %d, want 2", client.calls)
	}
	for i, prompt := range client.prompts {
		for _, want := range []string{
			"長期間無音だった",
			"独り言",
			`{"items":[]}`,
		} {
			if !strings.Contains(prompt, want) {
				t.Fatalf("prompt[%d] = %q, want it to contain %q", i, prompt, want)
			}
		}
	}
	if !strings.Contains(client.prompts[1], "直前の応答はJSON timeline契約違反でした") {
		t.Fatalf("retry prompt = %q, want retry instruction", client.prompts[1])
	}
}

func TestStageDoesNotAddIdleFollowupInstructionWhenRequestIsExplicit(t *testing.T) {
	client := &fakeClient{responses: []string{`{"items":[{"type":"speech","text":"どうしたの？"}]}`}}
	st := &stage{
		client:       client,
		agentStatus:  fakeAgentStatus{status: agentstatus.StatusIdle},
		systemPrompt: buildSystemPrompt("", nil),
	}

	items, err := st.requestTimeline(context.Background(), types.LLMRequest{
		RequestID:    "current",
		Role:         types.RoleUser,
		Text:         "電気つけて",
		GenerationID: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	if strings.Contains(client.prompts[0], "長期間無音だった") {
		t.Fatalf("prompt = %q, want no idle followup instruction", client.prompts[0])
	}
}

func TestStageAddsTimerSnapshotToPrompt(t *testing.T) {
	now := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	store := timerstate.NewStoreWithClock(func() time.Time { return now })
	timer := store.Create(now.Add(time.Hour), "起こす")
	client := &fakeClient{responses: []string{`{"items":[]}`}}
	st := &stage{
		client:       client,
		timers:       store,
		systemPrompt: buildSystemPrompt("base", nil),
	}

	_, err := st.requestTimeline(context.Background(), types.LLMRequest{
		RequestID:    "req-1",
		Role:         types.RoleUser,
		Text:         "確認して",
		GenerationID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt := client.prompts[0]
	for _, want := range []string{
		"現在の未到達タイマー一覧:",
		timer.ID,
		timer.At.Format(time.RFC3339),
		"action=起こす",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want it to contain %q", prompt, want)
		}
	}
}

func TestStageLogsNoResponseWithIdleReason(t *testing.T) {
	var logs bytes.Buffer
	prevOutput := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(prevOutput)

	client := &fakeClient{responses: []string{`{"items":[]}`}}
	st := &stage{
		client:       client,
		agentStatus:  fakeAgentStatus{status: agentstatus.StatusIdle},
		systemPrompt: buildSystemPrompt("", nil),
	}

	_, err := st.requestTimeline(context.Background(), types.LLMRequest{
		RequestID:    "req-1",
		Role:         types.RoleUser,
		Text:         "あー",
		GenerationID: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	logText := logs.String()
	for _, want := range []string{
		"llm: no response",
		"generation=7",
		"request_id=req-1",
		"reason=idle_candidate",
		`text="あー"`,
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log = %q, want it to contain %q", logText, want)
		}
	}
}

func TestStageRetriesInvalidResponse(t *testing.T) {
	var logs bytes.Buffer
	prevOutput := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(prevOutput)

	history := conversationhistory.NewStore()
	history.Append(types.ConversationRecord{Role: types.RoleUser, Text: "温度見て", GenerationID: 1})
	client := &fakeClient{responses: []string{
		`{"items":[{"type":"speech","text":""}]}`,
		`{"items":[{"type":"speech","text":"確認するね"}]}`,
	}}
	st, err := NewStage(Config{History: history, Client: client})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{Kind: types.EventLLMRequest, Payload: types.LLMRequest{RequestID: "req-1", GenerationID: 1}}
	select {
	case evt := <-st.Downstream:
		if evt.Kind != types.EventTimelineItem {
			t.Fatalf("Kind = %s", evt.Kind)
		}
		item := evt.Payload.(types.TimelineItem)
		if item.Text != "確認するね" {
			t.Fatalf("Text = %q", item.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting timeline item")
	}
	if client.calls != 2 {
		t.Fatalf("calls = %d, want 2", client.calls)
	}
	logText := logs.String()
	for _, want := range []string{
		"llm: invalid timeline response",
		"generation=1",
		"request_id=req-1",
		"attempt=1/10",
		"err=speech text is required",
		`raw_preview="{\"type\":\"speech\",\"text\":\"\"}"`,
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log = %q, want it to contain %q", logText, want)
		}
	}
}

func TestStagePrependsMemoryContextMessages(t *testing.T) {
	history := conversationhistory.NewStore()
	history.Append(types.ConversationRecord{Role: types.RoleUser, Text: "朝食を考えて", GenerationID: 1})
	client := &fakeClient{responses: []string{`{"items":[]}`}}
	memory := &fakeMemoryContextProvider{
		messages: []types.ChatMessage{{Role: types.RoleSystem, Content: `{"type":"memory_context","memories":[{"content":"朝はコーヒー"}]}`}},
	}
	st := &stage{
		client:       client,
		history:      history,
		memory:       memory,
		systemPrompt: buildSystemPrompt("", nil),
	}

	_, err := st.requestTimeline(context.Background(), types.LLMRequest{
		RequestID:    "req-1",
		GenerationID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if memory.calls != 1 {
		t.Fatalf("memory calls = %d, want 1", memory.calls)
	}
	if len(memory.records) != 1 || memory.records[0].Text != "朝食を考えて" {
		t.Fatalf("memory records = %#v", memory.records)
	}
	got := client.messages[0]
	if len(got) != 2 {
		t.Fatalf("messages len = %d, want 2", len(got))
	}
	if got[0].Content != memory.messages[0].Content {
		t.Fatalf("first message = %#v, want memory context", got[0])
	}
	if got[1].Role != types.RoleUser || !strings.Contains(got[1].Content, "朝食を考えて") {
		t.Fatalf("history message = %#v", got[1])
	}
}

func TestStageFallsBackWhenMemoryContextFails(t *testing.T) {
	var logs bytes.Buffer
	prevOutput := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(prevOutput)

	history := conversationhistory.NewStore()
	history.Append(types.ConversationRecord{Role: types.RoleUser, Text: "朝食を考えて", GenerationID: 1})
	client := &fakeClient{responses: []string{`{"items":[]}`}}
	memory := &fakeMemoryContextProvider{err: errors.New("embedding unavailable")}
	st := &stage{
		client:       client,
		history:      history,
		memory:       memory,
		systemPrompt: buildSystemPrompt("", nil),
	}

	_, err := st.requestTimeline(context.Background(), types.LLMRequest{
		RequestID:    "req-1",
		GenerationID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if memory.calls != 1 {
		t.Fatalf("memory calls = %d, want 1", memory.calls)
	}
	got := client.messages[0]
	if len(got) != 1 {
		t.Fatalf("messages len = %d, want history only", len(got))
	}
	logText := logs.String()
	for _, want := range []string{
		"llm: memory context unavailable",
		"generation=1",
		"request_id=req-1",
		"embedding unavailable",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log = %q, want it to contain %q", logText, want)
		}
	}
}

func TestStageBuildsMemoryContextOnceAcrossRetries(t *testing.T) {
	history := conversationhistory.NewStore()
	history.Append(types.ConversationRecord{Role: types.RoleUser, Text: "温度見て", GenerationID: 1})
	client := &fakeClient{responses: []string{
		`{"items":[{"type":"speech","text":""}]}`,
		`{"items":[{"type":"speech","text":"確認するね"}]}`,
	}}
	memory := &fakeMemoryContextProvider{
		messages: []types.ChatMessage{{Role: types.RoleSystem, Content: `{"type":"memory_context","memories":[{"content":"寒がり"}]}`}},
	}
	st := &stage{
		client:       client,
		history:      history,
		memory:       memory,
		systemPrompt: buildSystemPrompt("", nil),
	}

	_, err := st.requestTimeline(context.Background(), types.LLMRequest{
		RequestID:    "req-1",
		GenerationID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 2 {
		t.Fatalf("client calls = %d, want 2", client.calls)
	}
	if memory.calls != 1 {
		t.Fatalf("memory calls = %d, want 1", memory.calls)
	}
	if len(client.messages) != 2 || len(client.messages[0]) != 2 || len(client.messages[1]) != 2 {
		t.Fatalf("client messages = %#v", client.messages)
	}
	if client.messages[0][0].Content != client.messages[1][0].Content {
		t.Fatalf("memory context differs across retries: %#v", client.messages)
	}
}

func TestAppendRetryInstructionIncludesRawPreview(t *testing.T) {
	prompt := appendRetryInstruction("base prompt", errors.New("invalid timeline json"), "うん続けて、聞いてるよ")
	for _, want := range []string{
		"base prompt",
		"通常の文章を絶対に出力しないでください",
		"正しい出力例:",
		`{"items":[{"type":"speech","text":"うん、聞いてるよ"}`,
		"悪い出力例:",
		"直前に出力した不正な内容:",
		"うん続けて、聞いてるよ",
		"違反理由:\ninvalid timeline json",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt = %q, want it to contain %q", prompt, want)
		}
	}
}

func TestRawLinePreviewTruncatesRunes(t *testing.T) {
	rawLine := strings.Repeat("あ", maxRawLinePreviewRunes+1)
	preview := rawPreviewText(rawLine)
	if got := len([]rune(preview)); got != maxRawLinePreviewRunes+len([]rune(rawLinePreviewSuffix)) {
		t.Fatalf("preview rune length = %d, want %d", got, maxRawLinePreviewRunes+len([]rune(rawLinePreviewSuffix)))
	}
	if !strings.HasSuffix(preview, rawLinePreviewSuffix) {
		t.Fatalf("preview = %q, want suffix %q", preview, rawLinePreviewSuffix)
	}
	if !strings.Contains(preview, "あ") {
		t.Fatalf("preview = %q, want multibyte text preserved", preview)
	}
}

func TestRawLinePreviewFromNonParseErrorIsEmpty(t *testing.T) {
	if got := rawPreviewFromError(os.ErrNotExist); got != "" {
		t.Fatalf("preview = %q, want empty", got)
	}
}

type fakeMemoryContextProvider struct {
	calls    int
	records  []types.ConversationRecord
	messages []types.ChatMessage
	err      error
}

func (f *fakeMemoryContextProvider) BuildContext(ctx context.Context, records []types.ConversationRecord) ([]types.ChatMessage, error) {
	f.calls++
	f.records = append([]types.ConversationRecord(nil), records...)
	if f.err != nil {
		return nil, f.err
	}
	return append([]types.ChatMessage(nil), f.messages...), nil
}
