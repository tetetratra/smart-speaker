package conversation

import "testing"

func TestParseAIOutput(t *testing.T) {
	t.Run("speech 1行のみでも正常に解釈できる", func(t *testing.T) {
		out, ok := parseAIOutput(`{"type":"speech","text":"こんにちは"}`)
		if !ok {
			t.Fatal("parseAIOutput returned false")
		}
		if len(out.Timeline) != 1 {
			t.Fatalf("timeline len = %d, want 1", len(out.Timeline))
		}
	})

	t.Run("whiteboard chunkは無効", func(t *testing.T) {
		if _, ok := parseAIOutput("{\"type\":\"speech\",\"text\":\"こんにちは\"}\n{\"type\":\"whiteboard\",\"content\":\"- 10:00 会議\"}"); ok {
			t.Fatal("parseAIOutput returned true, want false")
		}
	})

	t.Run("timeline objectは無効", func(t *testing.T) {
		if _, ok := parseAIOutput(`{"timeline":[{"type":"speech","text":"こんにちは"}]}`); ok {
			t.Fatal("parseAIOutput returned true, want false")
		}
	})

	t.Run("NDJSON形式も解釈できる", func(t *testing.T) {
		out, ok := parseAIOutput("{\"type\":\"speech\",\"text\":\"こんにちは\"}\n{\"type\":\"wait\",\"sec\":1}\n{\"type\":\"speech\",\"text\":\"元気？\"}")
		if !ok {
			t.Fatal("parseAIOutput returned false")
		}
		if len(out.Timeline) != 3 {
			t.Fatalf("timeline len = %d, want 3", len(out.Timeline))
		}
	})

	t.Run("plain text 1行は無効", func(t *testing.T) {
		if _, ok := parseAIOutput("こんにちは"); ok {
			t.Fatal("parseAIOutput returned true, want false")
		}
	})

	t.Run("toolは末尾なら正常に解釈できる", func(t *testing.T) {
		out, ok := parseAIOutput("{\"type\":\"speech\",\"text\":\"確認するね\"}\n{\"type\":\"tool\",\"name\":\"get_temp\",\"args\":{}}")
		if !ok {
			t.Fatal("parseAIOutput returned false")
		}
		if len(out.Timeline) != 2 || out.Timeline[1].Type != "tool" {
			t.Fatalf("timeline = %+v", out.Timeline)
		}
	})

	t.Run("tool後のspeechは無効", func(t *testing.T) {
		if _, ok := parseAIOutput("{\"type\":\"tool\",\"name\":\"get_temp\",\"args\":{}}\n{\"type\":\"speech\",\"text\":\"28度だったよ\"}"); ok {
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

	t.Run("tool segmentを構築する", func(t *testing.T) {
		out := aiOutput{
			Timeline: []aiSegment{
				{Type: "speech", Text: "確認するね"},
				{Type: "tool", Name: "get_temp", Args: []byte(`{"room":"living"}`)},
			},
		}

		got := buildTimelineSegments(out)
		if len(got) != 2 {
			t.Fatalf("timeline len = %d, want 2", len(got))
		}
		if got[1].Type != "tool" || got[1].Tool == nil || got[1].Tool.Name != "get_temp" {
			t.Fatalf("tool segment = %+v", got[1])
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

	t.Run("未知typeは無効", func(t *testing.T) {
		if _, ok := parseAIChunk(`{"type":"unknown","text":"x"}`); ok {
			t.Fatal("parseAIChunk returned true, want false")
		}
	})

	t.Run("必須field欠落は無効", func(t *testing.T) {
		cases := []string{
			`{"type":"speech","text":" "}`,
			`{"type":"wait"}`,
			`{"type":"tool"}`,
		}
		for _, tc := range cases {
			if _, ok := parseAIChunk(tc); ok {
				t.Fatalf("parseAIChunk(%s) returned true, want false", tc)
			}
		}
	})
}

func TestParseAIChunks(t *testing.T) {
	t.Run("timeline objectは無効", func(t *testing.T) {
		if _, ok := parseAIChunks(`{"timeline":[{"type":"speech","text":"こんにちは"},{"type":"wait","sec":1}]}`); ok {
			t.Fatal("parseAIChunks returned true, want false")
		}
	})

	t.Run("plain textは無効", func(t *testing.T) {
		if _, ok := parseAIChunks("うん、わかった、だよ"); ok {
			t.Fatal("parseAIChunks returned true, want false")
		}
	})
}
