package memory

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestCreatorHookPassesSnapshotToCandidateCreator(t *testing.T) {
	history := &fakeHistory{records: []types.ConversationRecord{{ID: "rec-1", Role: types.RoleUser, Text: "朝はコーヒー"}}}
	creator := &fakeCandidateCreator{candidates: []Candidate{{Content: "ユーザーは朝にコーヒーを飲む", Tags: []string{"coffee"}}}}
	embedder := &fakeEmbedder{embeddings: map[string][]float64{"ユーザーは朝にコーヒーを飲む coffee": {0.1, 0.2}}}
	store := &fakeMemoryUpserter{}
	hook, err := NewCreatorHook(CreatorHookConfig{
		History:                history,
		CandidateCreator:       creator,
		Embedder:               embedder,
		Memory:                 store,
		DuplicateMinSimilarity: 0.98,
	})
	if err != nil {
		t.Fatalf("NewCreatorHook() error = %v", err)
	}

	if err := hook.Exec(context.Background()); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if !reflect.DeepEqual(creator.records, history.records) {
		t.Fatalf("creator records = %#v, want %#v", creator.records, history.records)
	}
	if got, want := embedder.inputs, []string{"ユーザーは朝にコーヒーを飲む coffee"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("embed inputs = %#v, want %#v", got, want)
	}
	if len(store.inputs) != 1 {
		t.Fatalf("upsert count = %d, want 1", len(store.inputs))
	}
	input := store.inputs[0]
	if input.Content != "ユーザーは朝にコーヒーを飲む" {
		t.Fatalf("Content = %q", input.Content)
	}
	if !reflect.DeepEqual(input.Tags, []string{"coffee"}) {
		t.Fatalf("Tags = %#v", input.Tags)
	}
	if !reflect.DeepEqual(input.Embedding, []float64{0.1, 0.2}) {
		t.Fatalf("Embedding = %#v", input.Embedding)
	}
	if input.DuplicateMinSimilarity != 0.98 {
		t.Fatalf("DuplicateMinSimilarity = %f, want 0.98", input.DuplicateMinSimilarity)
	}
}

func TestCreatorHookSkipsEmptyHistoryAndEmptyCandidates(t *testing.T) {
	t.Run("empty history", func(t *testing.T) {
		creator := &fakeCandidateCreator{}
		hook := mustCreatorHook(t, &fakeHistory{}, creator, &fakeEmbedder{}, &fakeMemoryUpserter{})
		if err := hook.Exec(context.Background()); err != nil {
			t.Fatalf("Exec() error = %v", err)
		}
		if creator.called {
			t.Fatal("candidate creator was called")
		}
	})

	t.Run("empty candidates", func(t *testing.T) {
		embedder := &fakeEmbedder{}
		store := &fakeMemoryUpserter{}
		hook := mustCreatorHook(t, &fakeHistory{records: []types.ConversationRecord{{Role: types.RoleUser, Text: "雑談"}}}, &fakeCandidateCreator{}, embedder, store)
		if err := hook.Exec(context.Background()); err != nil {
			t.Fatalf("Exec() error = %v", err)
		}
		if len(embedder.inputs) != 0 {
			t.Fatalf("embed inputs = %#v, want empty", embedder.inputs)
		}
		if len(store.inputs) != 0 {
			t.Fatalf("upsert inputs = %#v, want empty", store.inputs)
		}
	})
}

func TestCreatorHookReturnsCandidateCreatorError(t *testing.T) {
	wantErr := errors.New("openai failed")
	hook := mustCreatorHook(
		t,
		&fakeHistory{records: []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}}},
		&fakeCandidateCreator{err: wantErr},
		&fakeEmbedder{},
		&fakeMemoryUpserter{},
	)
	err := hook.Exec(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Exec() error = %v, want %v", err, wantErr)
	}
}

func TestCreatorHookContinuesCandidateErrors(t *testing.T) {
	embedErr := errors.New("embed failed")
	upsertErr := errors.New("upsert failed")
	embedder := &fakeEmbedder{
		embeddings: map[string][]float64{
			"保存できる記憶 ok":   {1, 0},
			"保存で失敗する記憶 ng": {0, 1},
		},
		errs: map[string]error{
			"embeddingで失敗する記憶 fail": embedErr,
		},
	}
	store := &fakeMemoryUpserter{errByContent: map[string]error{"保存で失敗する記憶": upsertErr}}
	hook := mustCreatorHook(
		t,
		&fakeHistory{records: []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}}},
		&fakeCandidateCreator{candidates: []Candidate{
			{Content: "保存できる記憶", Tags: []string{"ok"}},
			{Content: "embeddingで失敗する記憶", Tags: []string{"fail"}},
			{Content: "保存で失敗する記憶", Tags: []string{"ng"}},
		}},
		embedder,
		store,
	)

	err := hook.Exec(context.Background())
	if !errors.Is(err, embedErr) || !errors.Is(err, upsertErr) {
		t.Fatalf("Exec() error = %v, want joined embed and upsert errors", err)
	}
	if got, want := len(embedder.inputs), 3; got != want {
		t.Fatalf("embed count = %d, want %d", got, want)
	}
	if got, want := len(store.inputs), 2; got != want {
		t.Fatalf("upsert count = %d, want %d", got, want)
	}
	if store.inputs[0].Content != "保存できる記憶" {
		t.Fatalf("first upsert = %#v", store.inputs[0])
	}
	if store.inputs[1].Content != "保存で失敗する記憶" {
		t.Fatalf("second upsert = %#v", store.inputs[1])
	}
}

func TestNewCreatorHookValidatesConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  CreatorHookConfig
		want string
	}{
		{name: "history", cfg: CreatorHookConfig{}, want: "history is required"},
		{name: "candidate creator", cfg: CreatorHookConfig{History: &fakeHistory{}}, want: "candidate creator is required"},
		{name: "embedder", cfg: CreatorHookConfig{History: &fakeHistory{}, CandidateCreator: &fakeCandidateCreator{}}, want: "embedder is required"},
		{name: "memory", cfg: CreatorHookConfig{History: &fakeHistory{}, CandidateCreator: &fakeCandidateCreator{}, Embedder: &fakeEmbedder{}}, want: "memory is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCreatorHook(tt.cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func mustCreatorHook(t *testing.T, history *fakeHistory, creator *fakeCandidateCreator, embedder *fakeEmbedder, store *fakeMemoryUpserter) *CreatorHook {
	t.Helper()
	hook, err := NewCreatorHook(CreatorHookConfig{
		History:          history,
		CandidateCreator: creator,
		Embedder:         embedder,
		Memory:           store,
	})
	if err != nil {
		t.Fatalf("NewCreatorHook() error = %v", err)
	}
	return hook
}

type fakeHistory struct {
	records []types.ConversationRecord
}

func (f *fakeHistory) Snapshot() []types.ConversationRecord {
	out := make([]types.ConversationRecord, len(f.records))
	copy(out, f.records)
	return out
}

type fakeCandidateCreator struct {
	called     bool
	records    []types.ConversationRecord
	candidates []Candidate
	err        error
}

func (f *fakeCandidateCreator) CreateCandidates(ctx context.Context, records []types.ConversationRecord) ([]Candidate, error) {
	f.called = true
	f.records = append([]types.ConversationRecord(nil), records...)
	if f.err != nil {
		return nil, f.err
	}
	return append([]Candidate(nil), f.candidates...), nil
}

type fakeEmbedder struct {
	inputs     []string
	embeddings map[string][]float64
	errs       map[string]error
}

func (f *fakeEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	f.inputs = append(f.inputs, text)
	if err := f.errs[text]; err != nil {
		return nil, err
	}
	if embedding := f.embeddings[text]; embedding != nil {
		return append([]float64(nil), embedding...), nil
	}
	return []float64{0.1}, nil
}

type fakeMemoryUpserter struct {
	inputs       []memorystate.UpsertInput
	errByContent map[string]error
}

func (f *fakeMemoryUpserter) Upsert(input memorystate.UpsertInput) (memorystate.Record, memorystate.UpsertResult, error) {
	f.inputs = append(f.inputs, input)
	if err := f.errByContent[input.Content]; err != nil {
		return memorystate.Record{}, memorystate.UpsertResult{}, err
	}
	return memorystate.Record{Content: input.Content, Tags: input.Tags, Embedding: input.Embedding}, memorystate.UpsertResult{Created: true}, nil
}
