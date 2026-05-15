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

func TestBuildTimelineSegments(t *testing.T) {
	t.Run("speechのURLとcitationを除去する", func(t *testing.T) {
		out := aiOutput{
			Timeline: []aiSegment{
				{Type: "speech", Text: "確認して https://example.com [link](https://example.com) citeturn0search0"},
			},
		}

		got := buildTimelineSegments(out)
		if len(got) != 1 {
			t.Fatalf("timeline len = %d, want 1", len(got))
		}
		if got[0].Type != "speech" {
			t.Fatalf("segment type = %q, want speech", got[0].Type)
		}
		if got[0].Text != "確認して" {
			t.Fatalf("segment text = %q, want %q", got[0].Text, "確認して")
		}
	})

	t.Run("waitを0から5秒へ正規化する", func(t *testing.T) {
		neg := -1
		large := 9
		out := aiOutput{
			Timeline: []aiSegment{
				{Type: "wait", Sec: &neg},
				{Type: "wait", Sec: &large},
				{Type: "speech", Text: "こんにちは"},
			},
		}

		got := buildTimelineSegments(out)
		if len(got) != 3 {
			t.Fatalf("timeline len = %d, want 3", len(got))
		}
		if got[0].WaitSec != 0 {
			t.Fatalf("first wait = %d, want 0", got[0].WaitSec)
		}
		if got[1].WaitSec != 5 {
			t.Fatalf("second wait = %d, want 5", got[1].WaitSec)
		}
	})
}

func TestParseAIChunk(t *testing.T) {
	t.Run("speech chunkを解釈できる", func(t *testing.T) {
		chunk, ok := parseAIChunk(`{"type":"speech","text":"こんにちは"}`)
		if !ok {
			t.Fatal("parseAIChunk returned false")
		}
		if chunk.Type != "speech" || chunk.Text != "こんにちは" {
			t.Fatalf("chunk = %+v", chunk)
		}
	})

	t.Run("wait chunkを解釈できる", func(t *testing.T) {
		chunk, ok := parseAIChunk(`{"type":"wait","sec":2}`)
		if !ok {
			t.Fatal("parseAIChunk returned false")
		}
		if chunk.Type != "wait" || chunk.Sec == nil || *chunk.Sec != 2 {
			t.Fatalf("chunk = %+v", chunk)
		}
	})

	t.Run("whiteboard chunkを解釈できる", func(t *testing.T) {
		chunk, ok := parseAIChunk(`{"type":"whiteboard","content":"- 10:00 会議"}`)
		if !ok {
			t.Fatal("parseAIChunk returned false")
		}
		if chunk.Type != "whiteboard" || chunk.Content != "- 10:00 会議" {
			t.Fatalf("chunk = %+v", chunk)
		}
	})

	t.Run("未知typeは無効", func(t *testing.T) {
		if _, ok := parseAIChunk(`{"type":"unknown","text":"x"}`); ok {
			t.Fatal("parseAIChunk returned true, want false")
		}
	})

	t.Run("必須field欠落は無効", func(t *testing.T) {
		cases := []string{
			`{"type":"speech","text":" "}`,
			`{"type":"wait"}`,
			`{"type":"whiteboard","content":" "}`,
		}
		for _, tc := range cases {
			if _, ok := parseAIChunk(tc); ok {
				t.Fatalf("parseAIChunk(%s) returned true, want false", tc)
			}
		}
	})
}
