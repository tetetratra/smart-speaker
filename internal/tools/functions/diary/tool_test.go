package diary

import (
	"strings"
	"testing"
	"time"
)

type stubAppender struct {
	when    time.Time
	content string
	path    string
	err     error
}

func (s *stubAppender) AppendEntry(when time.Time, content string) (string, error) {
	s.when = when
	s.content = content
	if s.err != nil {
		return "", s.err
	}
	return s.path, nil
}

func TestToolRun(t *testing.T) {
	appender := &stubAppender{path: "data/diary.md"}
	tool := New(appender)

	out, err := tool.Run(map[string]any{
		"content": "一行目\\n二行目",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(appender.content, "一行目\n二行目") {
		t.Fatalf("content = %q", appender.content)
	}
	if appender.when.IsZero() {
		t.Fatal("when is zero")
	}
	if out["path"] != "data/diary.md" {
		t.Fatalf("path = %#v", out["path"])
	}
	if _, ok := out["timestamp"].(string); !ok {
		t.Fatalf("timestamp = %#v", out["timestamp"])
	}
}
