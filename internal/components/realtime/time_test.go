package realtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/eventqueue"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
)

// 日時は指示ではなく会話の末尾に応答ごとに足し、指示を応答ごとに変えない。
func TestResponseAddsTimeNoticeInsteadOfChangingInstructions(t *testing.T) {
	commands := eventqueue.New[any](nil)
	s := &stage{
		cfg:       Config{Generation: generation.NewStore()},
		ready:     true,
		transport: &transport{commands: commands},
	}
	pop := func() map[string]any {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		command, ok := commands.Pop(ctx)
		if !ok {
			t.Fatal("送信がありません")
		}
		return command.(map[string]any)
	}
	before := time.Now()
	s.wantResponse = true
	s.maybeRespond(context.Background())
	notice := pop()
	item := notice["item"].(map[string]any)
	if notice["type"] != "conversation.item.create" || item["role"] != "system" {
		t.Fatalf("日時の知らせがありません: %v", notice)
	}
	text := item["content"].([]any)[0].(map[string]any)["text"].(string)
	_, timestamp, _ := strings.Cut(text, "この時点の日時: ")
	timestamp, _, _ = strings.Cut(timestamp, "\n")
	now, err := time.Parse(time.RFC3339, timestamp)
	if err != nil || now.Before(before.Truncate(time.Second)) || now.After(time.Now()) {
		t.Fatalf("日時が違います: %s, %v", text, err)
	}
	response := pop()
	if _, changed := response["response"].(map[string]any)["instructions"]; response["type"] != "response.create" || changed {
		t.Fatalf("指示が変わっています: %v", response)
	}

	// 次の応答でも、前の知らせは消さずに新しい知らせを足す。
	s.active = ""
	s.wantResponse = true
	s.maybeRespond(context.Background())
	if next := pop(); next["type"] != "conversation.item.create" {
		t.Fatalf("次の応答の前に日時を足していません: %v", next)
	}
	if next := pop(); next["type"] != "response.create" {
		t.Fatalf("応答要求がありません: %v", next)
	}
}
