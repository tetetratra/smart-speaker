package realtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/eventqueue"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
)

func TestResponseRefreshesTimeWithoutReusingSessionTimestamp(t *testing.T) {
	commands := eventqueue.New[any](nil)
	s := &stage{
		cfg:          Config{Generation: generation.NewStore()},
		ready:        true,
		wantResponse: true,
		instructions: "元の指示と再接続時の履歴",
		transport:    &transport{commands: commands},
		started:      time.Date(2024, 11, 5, 0, 0, 0, 0, time.UTC),
	}
	before := time.Now()
	s.maybeRespond(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	command, ok := commands.Pop(ctx)
	if !ok {
		t.Fatal("応答要求がありません")
	}
	response := command.(map[string]any)["response"].(map[string]any)
	instructions := response["instructions"].(string)
	if !strings.Contains(instructions, "元の指示と再接続時の履歴") {
		t.Fatalf("元の指示が失われました: %s", instructions)
	}
	_, timestamp, ok := strings.Cut(instructions, "現在日時: ")
	if !ok {
		t.Fatal("現在日時がありません")
	}
	timestamp, _, _ = strings.Cut(timestamp, "\n")
	now, err := time.Parse(time.RFC3339, timestamp)
	if err != nil || now.Before(before.Truncate(time.Second)) || now.After(time.Now()) {
		t.Fatalf("応答に古い日時が含まれています: %s, %v", timestamp, err)
	}
}
