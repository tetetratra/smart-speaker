package whiteboard

import (
	"testing"

	types "smart-speaker/internal/types"
)

func TestToolRunEmitsWhiteboardUpdate(t *testing.T) {
	tool := New()
	var got []types.Event
	tool.SetEventEmitter(func(evt types.Event) {
		got = append(got, evt)
	})

	out, err := tool.Run(map[string]any{
		"content": `  - 10:00 会議\n- 13:00 歯医者  `,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if out["updated"] != true {
		t.Fatalf("out = %#v", out)
	}
	if out["content"] != "- 10:00 会議\n- 13:00 歯医者" {
		t.Fatalf("content = %#v", out["content"])
	}
	if len(got) != 1 {
		t.Fatalf("events len = %d, want 1", len(got))
	}
	if got[0].Kind != types.EventWhiteboardUpdate {
		t.Fatalf("event kind = %s", got[0].Kind)
	}
	update, ok := got[0].Payload.(types.WhiteboardUpdate)
	if !ok {
		t.Fatalf("payload type = %T", got[0].Payload)
	}
	if update.Content != "- 10:00 会議\n- 13:00 歯医者" {
		t.Fatalf("update content = %q", update.Content)
	}
}

func TestToolRunRequiresContent(t *testing.T) {
	tool := New()

	if _, err := tool.Run(map[string]any{"content": "   "}); err == nil {
		t.Fatal("Run() error = nil, want error")
	}
}
