package whiteboard

import (
	"context"
	"testing"
)

func TestToolRunReturnsContent(t *testing.T) {
	tool := New()

	out, err := tool.Run(context.Background(), map[string]any{
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

}

func TestToolRunRequiresContent(t *testing.T) {
	tool := New()

	if _, err := tool.Run(context.Background(), map[string]any{"content": "   "}); err == nil {
		t.Fatal("Run() error = nil, want error")
	}
}
