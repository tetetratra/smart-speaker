package sessionreset

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type hookFunc func(context.Context) error

func (f hookFunc) Exec(ctx context.Context) error {
	return f(ctx)
}

func TestStageResetsAfterIdleTimeout(t *testing.T) {
	history := conversationhistory.NewStore()
	history.Append(types.ConversationRecord{ID: "prev", Role: types.RoleUser, Text: "電気つけて", GenerationID: 1})
	generationStore := generation.NewStore()
	generationStore.Next()

	var mu sync.Mutex
	var calls int
	hook := hookFunc(func(ctx context.Context) error {
		if len(history.Snapshot()) != 1 {
			t.Fatalf("history was reset before hook")
		}
		if got := generationStore.Current(); got != 1 {
			t.Fatalf("generation before hook = %d, want 1", got)
		}
		mu.Lock()
		calls++
		mu.Unlock()
		return nil
	})
	st := NewStage(Config{
		IdleTimeout: 20 * time.Millisecond,
		History:     history,
		Generation:  generationStore,
		Hooks:       []Hook{hook},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- userCommitEvent("current", 1)

	waitUntil(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return calls == 1 && len(history.Snapshot()) == 0 && generationStore.Current() == 2
	})
}

func TestStageExtendsTimerOnUserActivity(t *testing.T) {
	history := conversationhistory.NewStore()
	history.Append(types.ConversationRecord{ID: "prev", Role: types.RoleUser, Text: "電気つけて", GenerationID: 1})
	generationStore := generation.NewStore()
	generationStore.Next()
	st := NewStage(Config{
		IdleTimeout: 80 * time.Millisecond,
		History:     history,
		Generation:  generationStore,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- userCommitEvent("first", 1)
	time.Sleep(40 * time.Millisecond)
	st.Upstream <- userCommitEvent("second", 1)
	time.Sleep(50 * time.Millisecond)

	if got := generationStore.Current(); got != 1 {
		t.Fatalf("generation reset too early = %d, want 1", got)
	}
	if got := len(history.Snapshot()); got != 1 {
		t.Fatalf("history len after timer extension = %d, want 1", got)
	}
	waitUntil(t, time.Second, func() bool {
		return len(history.Snapshot()) == 0 && generationStore.Current() == 2
	})
}

func TestStageContinuesWhenHookFails(t *testing.T) {
	history := conversationhistory.NewStore()
	history.Append(types.ConversationRecord{ID: "prev", Role: types.RoleUser, Text: "電気つけて", GenerationID: 1})
	generationStore := generation.NewStore()
	generationStore.Next()

	var mu sync.Mutex
	var calls []string
	st := NewStage(Config{
		IdleTimeout: 20 * time.Millisecond,
		History:     history,
		Generation:  generationStore,
		Hooks: []Hook{
			hookFunc(func(ctx context.Context) error {
				mu.Lock()
				calls = append(calls, "first")
				mu.Unlock()
				return errors.New("hook failed")
			}),
			hookFunc(func(ctx context.Context) error {
				mu.Lock()
				calls = append(calls, "second")
				mu.Unlock()
				return nil
			}),
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- userCommitEvent("current", 1)

	waitUntil(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(calls) == 2 && len(history.Snapshot()) == 0 && generationStore.Current() == 2
	})
	mu.Lock()
	defer mu.Unlock()
	if calls[0] != "first" || calls[1] != "second" {
		t.Fatalf("calls = %#v, want first then second", calls)
	}
}

func TestStageDisabledWhenIdleTimeoutIsNonPositive(t *testing.T) {
	history := conversationhistory.NewStore()
	history.Append(types.ConversationRecord{ID: "prev", Role: types.RoleUser, Text: "電気つけて", GenerationID: 1})
	generationStore := generation.NewStore()
	generationStore.Next()
	st := NewStage(Config{
		IdleTimeout: 0,
		History:     history,
		Generation:  generationStore,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- userCommitEvent("current", 1)
	time.Sleep(50 * time.Millisecond)

	if got := len(history.Snapshot()); got != 1 {
		t.Fatalf("history len = %d, want 1", got)
	}
	if got := generationStore.Current(); got != 1 {
		t.Fatalf("generation = %d, want 1", got)
	}
}

func TestStageIgnoresNonUserCommitRequest(t *testing.T) {
	history := conversationhistory.NewStore()
	history.Append(types.ConversationRecord{ID: "prev", Role: types.RoleUser, Text: "電気つけて", GenerationID: 1})
	generationStore := generation.NewStore()
	generationStore.Next()
	st := NewStage(Config{
		IdleTimeout: 20 * time.Millisecond,
		History:     history,
		Generation:  generationStore,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- types.Event{
		Kind: types.EventConversationCommitRequest,
		Payload: types.ConversationCommitRequest{
			Role:         types.RoleToolResult,
			Text:         "tool result",
			GenerationID: 1,
		},
	}
	time.Sleep(50 * time.Millisecond)

	if got := len(history.Snapshot()); got != 1 {
		t.Fatalf("history len = %d, want 1", got)
	}
	if got := generationStore.Current(); got != 1 {
		t.Fatalf("generation = %d, want 1", got)
	}
}

func userCommitEvent(text string, generationID types.GenerationID) types.Event {
	return types.Event{
		Kind: types.EventConversationCommitRequest,
		Payload: types.ConversationCommitRequest{
			Role:         types.RoleUser,
			Text:         text,
			GenerationID: generationID,
		},
	}
}

func waitUntil(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if ok() {
		return
	}
	t.Fatal("condition was not met before timeout")
}
