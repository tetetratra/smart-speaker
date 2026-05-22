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

	"smart-speaker/internal/states/conversationhistory"
	types "smart-speaker/internal/types"
)

type fakeClient struct {
	calls     int
	responses []string
	prompts   []string
}

func (f *fakeClient) CreateResponseStream(ctx context.Context, messages []types.ChatMessage, systemContent string) (string, error) {
	f.prompts = append(f.prompts, systemContent)
	idx := f.calls
	f.calls++
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	return f.responses[idx], nil
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
