package llm

import (
	"bytes"
	"context"
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
	responses [][]string
}

func (f *fakeClient) CreateResponseStream(ctx context.Context, messages []types.ChatMessage, systemContent string, onLine func(string) error) error {
	idx := f.calls
	f.calls++
	if idx >= len(f.responses) {
		idx = len(f.responses) - 1
	}
	for _, line := range f.responses[idx] {
		if err := onLine(line); err != nil {
			return err
		}
	}
	return nil
}

func TestStageRetriesInvalidResponse(t *testing.T) {
	var logs bytes.Buffer
	prevOutput := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(prevOutput)

	history := conversationhistory.NewStore()
	history.Append(types.ConversationRecord{Role: types.RoleUser, Text: "温度見て", GenerationID: 1})
	client := &fakeClient{responses: [][]string{
		{`{"type":"speech","text":""}`},
		{`{"type":"speech","text":"確認するね"}`},
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
		"llm: invalid ndjson response",
		"generation=1",
		"request_id=req-1",
		"attempt=1/5",
		"err=speech text is required",
		`raw_line_preview="{\"type\":\"speech\",\"text\":\"\"}"`,
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log = %q, want it to contain %q", logText, want)
		}
	}
}

func TestRawLinePreviewTruncatesRunes(t *testing.T) {
	rawLine := strings.Repeat("あ", maxRawLinePreviewRunes+1)
	preview := rawLinePreview(rawLine)
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
	if got := rawLinePreviewFromError(os.ErrNotExist); got != "" {
		t.Fatalf("preview = %q, want empty", got)
	}
}
