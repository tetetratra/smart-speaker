package memory

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestContextProviderBuildsMemoryContextFromRecentConversation(t *testing.T) {
	embedder := &fakeEmbedder{embeddings: map[string][]float64{
		"agent: 少し待ってね\nuser: 明日の朝食どうしよう": {0.2, 0.8},
	}}
	memory := &fakeMemorySearcher{results: []memorystate.SearchResult{
		{Record: memorystate.Record{Content: "ユーザーは朝にコーヒーを飲む", Tags: []string{"coffee"}, Embedding: []float64{0.1}}},
		{Record: memorystate.Record{Content: "ユーザーは辛い料理が苦手"}},
	}}
	provider, err := NewContextProvider(ContextProviderConfig{
		Embedder:         embedder,
		Memory:           memory,
		QueryRecordLimit: 2,
		SearchLimit:      2,
		MinSimilarity:    0.82,
	})
	if err != nil {
		t.Fatalf("NewContextProvider() error = %v", err)
	}

	messages, err := provider.BuildContext(context.Background(), []types.ConversationRecord{
		{Role: types.RoleUser, Text: "昨日の夕飯は何だった？"},
		{Role: types.RoleToolCall, Text: `{"name":"calendar"}`},
		{Role: types.RoleToolResult, Text: `{"events":[]}`},
		{Role: types.RoleAgent, Text: "少し待ってね"},
		{Role: types.RoleUser, Text: "明日の朝食どうしよう"},
	})
	if err != nil {
		t.Fatalf("BuildContext() error = %v", err)
	}

	if got, want := embedder.inputs, []string{"agent: 少し待ってね\nuser: 明日の朝食どうしよう"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("embed inputs = %#v, want %#v", got, want)
	}
	if got, want := memory.queries, [][]float64{{0.2, 0.8}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("search queries = %#v, want %#v", got, want)
	}
	if got, want := memory.options, []memorystate.SearchOptions{{MinSimilarity: 0.82, Limit: 2}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("search options = %#v, want %#v", got, want)
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
	if got, want := len(payload.Memories), 2; got != want {
		t.Fatalf("memories len = %d, want %d", got, want)
	}
	if payload.Memories[0].Content != "ユーザーは朝にコーヒーを飲む" {
		t.Fatalf("memory[0].content = %q", payload.Memories[0].Content)
	}
	if strings.Contains(messages[0].Content, "coffee") || strings.Contains(messages[0].Content, "0.82") {
		t.Fatalf("message content = %s, want content only", messages[0].Content)
	}
}

func TestContextProviderSkipsEmptyQueryAndEmptyResults(t *testing.T) {
	t.Run("empty query", func(t *testing.T) {
		embedder := &fakeEmbedder{}
		memory := &fakeMemorySearcher{}
		provider := mustContextProvider(t, embedder, memory)

		messages, err := provider.BuildContext(context.Background(), []types.ConversationRecord{
			{Role: types.RoleToolCall, Text: `{"name":"timer"}`},
			{Role: types.RoleUser, Text: "  "},
		})
		if err != nil {
			t.Fatalf("BuildContext() error = %v", err)
		}
		if len(messages) != 0 {
			t.Fatalf("messages = %#v, want empty", messages)
		}
		if len(embedder.inputs) != 0 {
			t.Fatalf("embed inputs = %#v, want empty", embedder.inputs)
		}
		if len(memory.queries) != 0 {
			t.Fatalf("search queries = %#v, want empty", memory.queries)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		provider := mustContextProvider(t, &fakeEmbedder{}, &fakeMemorySearcher{})
		messages, err := provider.BuildContext(context.Background(), []types.ConversationRecord{
			{Role: types.RoleUser, Text: "朝食の話"},
		})
		if err != nil {
			t.Fatalf("BuildContext() error = %v", err)
		}
		if len(messages) != 0 {
			t.Fatalf("messages = %#v, want empty", messages)
		}
	})
}

func TestContextProviderReturnsEmbedError(t *testing.T) {
	wantErr := errors.New("embed failed")
	provider := mustContextProvider(t, &fakeEmbedder{errs: map[string]error{"user: test": wantErr}}, &fakeMemorySearcher{})

	_, err := provider.BuildContext(context.Background(), []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}})
	if !errors.Is(err, wantErr) {
		t.Fatalf("BuildContext() error = %v, want %v", err, wantErr)
	}
}

func TestNewContextProviderDefaultsAndValidation(t *testing.T) {
	provider, err := NewContextProvider(ContextProviderConfig{
		Embedder: &fakeEmbedder{},
		Memory:   &fakeMemorySearcher{},
	})
	if err != nil {
		t.Fatalf("NewContextProvider() error = %v", err)
	}
	if provider.queryRecordLimit != DefaultContextQueryRecordLimit {
		t.Fatalf("queryRecordLimit = %d", provider.queryRecordLimit)
	}
	if provider.searchLimit != DefaultContextSearchLimit {
		t.Fatalf("searchLimit = %d", provider.searchLimit)
	}
	if provider.minSimilarity != DefaultContextMinSimilarity {
		t.Fatalf("minSimilarity = %f", provider.minSimilarity)
	}

	tests := []struct {
		name string
		cfg  ContextProviderConfig
		want string
	}{
		{name: "embedder", cfg: ContextProviderConfig{}, want: "embedder is required"},
		{name: "memory", cfg: ContextProviderConfig{Embedder: &fakeEmbedder{}}, want: "memory is required"},
		{name: "similarity", cfg: ContextProviderConfig{Embedder: &fakeEmbedder{}, Memory: &fakeMemorySearcher{}, MinSimilarity: 1.1}, want: "min similarity must be <= 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewContextProvider(tt.cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func mustContextProvider(t *testing.T, embedder *fakeEmbedder, memory *fakeMemorySearcher) *ContextProvider {
	t.Helper()
	provider, err := NewContextProvider(ContextProviderConfig{
		Embedder: embedder,
		Memory:   memory,
	})
	if err != nil {
		t.Fatalf("NewContextProvider() error = %v", err)
	}
	return provider
}

type fakeMemorySearcher struct {
	queries [][]float64
	options []memorystate.SearchOptions
	results []memorystate.SearchResult
}

func (f *fakeMemorySearcher) Search(query []float64, opts memorystate.SearchOptions) []memorystate.SearchResult {
	f.queries = append(f.queries, append([]float64(nil), query...))
	f.options = append(f.options, opts)
	return append([]memorystate.SearchResult(nil), f.results...)
}
