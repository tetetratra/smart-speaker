package conversation

import (
	"context"
	"errors"
	"testing"

	types "smart-speaker/internal/types"
)

type stubDiaryReader struct {
	content string
	err     error
}

func (s stubDiaryReader) Content() (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.content, nil
}

func TestContextProviderWithDiaryContext(t *testing.T) {
	messages := []types.ChatMessage{{Role: "user", Content: "こんにちは"}}

	t.Run("diary がある場合は先頭に system message を追加する", func(t *testing.T) {
		p := newContextProvider(nil, stubDiaryReader{content: "# 2026-03-21 10:00\n内容"})

		got := p.WithSystemContexts(context.Background(), messages)

		if len(got) != 2 {
			t.Fatalf("messages len = %d, want 2", len(got))
		}
		if got[0].Role != "system" {
			t.Fatalf("first role = %q, want system", got[0].Role)
		}
		if got[0].Content != diaryPromptPrefix+"# 2026-03-21 10:00\n内容" {
			t.Fatalf("first content = %q", got[0].Content)
		}
	})

	t.Run("diary 読み取り失敗時は追加しない", func(t *testing.T) {
		p := newContextProvider(nil, stubDiaryReader{err: errors.New("boom")})

		got := p.WithSystemContexts(context.Background(), messages)

		if len(got) != 1 {
			t.Fatalf("messages len = %d, want 1", len(got))
		}
		if got[0] != messages[0] {
			t.Fatalf("message = %+v, want %+v", got[0], messages[0])
		}
	})
}
