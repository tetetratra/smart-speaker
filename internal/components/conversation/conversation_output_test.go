package conversation

import "testing"

func TestParseAIOutput(t *testing.T) {
	t.Run("timelineのみがある場合は正常に解釈できる", func(t *testing.T) {
		out, ok := parseAIOutput(`{"timeline":[{"type":"speech","text":"こんにちは"}]}`)
		if !ok {
			t.Fatal("parseAIOutput returned false")
		}
		if len(out.Timeline) != 1 {
			t.Fatalf("timeline len = %d, want 1", len(out.Timeline))
		}
		if out.Whiteboard != nil {
			t.Fatalf("whiteboard = %#v, want nil", out.Whiteboard)
		}
	})

	t.Run("timelineとwhiteboardがある場合はwhiteboardを取得できる", func(t *testing.T) {
		out, ok := parseAIOutput(`{"timeline":[{"type":"speech","text":"こんにちは"}],"whiteboard":{"content":"- 10:00 会議"}}`)
		if !ok {
			t.Fatal("parseAIOutput returned false")
		}
		if out.Whiteboard == nil {
			t.Fatal("whiteboard is nil")
		}
		if out.Whiteboard.Content != "- 10:00 会議" {
			t.Fatalf("whiteboard content = %q", out.Whiteboard.Content)
		}
	})

	t.Run("whiteboardが空文字の場合は無効", func(t *testing.T) {
		if _, ok := parseAIOutput(`{"timeline":[{"type":"speech","text":"こんにちは"}],"whiteboard":{"content":"   "}}`); ok {
			t.Fatal("parseAIOutput returned true, want false")
		}
	})

	t.Run("timelineが無効な場合は全体を無効として扱う", func(t *testing.T) {
		if _, ok := parseAIOutput(`{"timeline":[{"type":"wait"}],"whiteboard":{"content":"test"}}`); ok {
			t.Fatal("parseAIOutput returned true, want false")
		}
	})
}
