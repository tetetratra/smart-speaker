package memory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestContextProviderBuildsContextFromAllMemories(t *testing.T) {
	memory := &fakeMemoryReader{records: []memorystate.Record{
		{Content: "ユーザーは朝にコーヒーを飲む", Tags: []string{"coffee"}, Embedding: []float64{1, 0}},
		{Content: "ユーザーは辛い料理が苦手", Tags: []string{"food"}, Embedding: []float64{0, 1}},
		{Content: "ユーザーは週末にジョギングする", Tags: []string{"exercise"}, Embedding: []float64{-1, 0}},
		{Content: "ユーザーは猫を飼っている", Tags: []string{"pet"}},
	}}
	provider := mustContextProvider(t, memory)

	messages, err := provider.BuildContext(context.Background(), nil)
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	if memory.calls != 1 {
		t.Fatalf("Snapshot() calls = %d, want 1", memory.calls)
	}
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	if messages[0].Role != types.RoleSystem {
		t.Fatalf("message role = %q, want system", messages[0].Role)
	}
	var payload struct {
		Type     string `json:"type"`
		Memories []struct {
			Content string `json:"content"`
		} `json:"memories"`
	}
	if err := json.Unmarshal([]byte(messages[0].Content), &payload); err != nil {
		t.Fatalf("memory context JSON: %v", err)
	}
	if payload.Type != "memory_context" {
		t.Fatalf("type = %q, want memory_context", payload.Type)
	}
	if got, want := len(payload.Memories), len(memory.records); got != want {
		t.Fatalf("memories len = %d, want %d", got, want)
	}
	for i, record := range memory.records {
		if payload.Memories[i].Content != record.Content {
			t.Fatalf("memory[%d].content = %q, want %q", i, payload.Memories[i].Content, record.Content)
		}
	}
	if strings.Contains(messages[0].Content, "coffee") || strings.Contains(messages[0].Content, "embedding") {
		t.Fatalf("message content = %s, want content only", messages[0].Content)
	}
}

func TestContextProviderReturnsNoMessagesWithoutMemories(t *testing.T) {
	memory := &fakeMemoryReader{}
	provider := mustContextProvider(t, memory)

	messages, err := provider.BuildContext(context.Background(), []types.ConversationRecord{
		{Role: types.RoleUser, Text: "朝食の話"},
	})
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages = %#v, want empty", messages)
	}
	if memory.calls != 1 {
		t.Fatalf("Snapshot() calls = %d, want 1", memory.calls)
	}
}

func TestNewContextProviderRequiresMemory(t *testing.T) {
	_, err := NewContextProvider(ContextProviderConfig{})
	if err == nil || !strings.Contains(err.Error(), "memory is required") {
		t.Fatalf("err = %v, want it to contain memory is required", err)
	}
}

func mustContextProvider(t *testing.T, memory *fakeMemoryReader) *ContextProvider {
	t.Helper()
	provider, err := NewContextProvider(ContextProviderConfig{Memory: memory})
	if err != nil {
		t.Fatalf("NewContextProvider() error = %v", err)
	}
	return provider
}

type fakeMemoryReader struct {
	calls   int
	records []memorystate.Record
}

func (f *fakeMemoryReader) Snapshot() []memorystate.Record {
	f.calls++
	return append([]memorystate.Record(nil), f.records...)
}
