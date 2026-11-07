package memory

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestCreatorHookPassesSnapshotToOperationCreator(t *testing.T) {
	history := &fakeHistory{records: []types.ConversationRecord{{ID: "rec-1", Role: types.RoleUser, Text: "朝はコーヒー"}}}
	creator := &fakeOperationCreator{operations: []Operation{{Action: OperationAdd, Content: "ユーザーは朝にコーヒーを飲む", Tags: []string{"coffee"}}}}
	store := &fakeMemoryWriter{}
	hook := mustCreatorHook(t, history, creator, store)

	if err := hook.Exec(context.Background()); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if !reflect.DeepEqual(creator.records, history.records) {
		t.Fatalf("creator records = %#v, want %#v", creator.records, history.records)
	}
	if len(store.inputs) != 1 {
		t.Fatalf("upsert count = %d, want 1", len(store.inputs))
	}
	input := store.inputs[0]
	if input.Content != "ユーザーは朝にコーヒーを飲む" || !reflect.DeepEqual(input.Tags, []string{"coffee"}) {
		t.Fatalf("upsert input = %#v", input)
	}
}

func TestCreatorHookPassesExistingMemoriesAndAppliesOperations(t *testing.T) {
	creator := &fakeOperationCreator{operations: []Operation{
		{Action: OperationUpdate, TargetID: "1", Content: "ユーザーは横浜に住んでいる", Tags: []string{"location"}},
		{Action: OperationDelete, TargetID: "2"},
		{Action: OperationAdd, Content: "ユーザーは寿司が好き", Tags: []string{"food"}},
	}}
	store := &fakeMemoryWriter{records: []memorystate.Record{
		{ID: "1788926855608507735-3", Content: "ユーザーは大岡山に住んでいる"},
		{ID: "1789095948492476660-8", Content: "ユーザーは朝にコーヒーを飲む"},
	}}
	hook := mustCreatorHook(t, &fakeHistory{records: []types.ConversationRecord{{Role: types.RoleUser, Text: "横浜に引っ越した"}}}, creator, store)

	if err := hook.Exec(context.Background()); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	// LLM には長い store ID ではなく短い参照番号を渡す。
	wantExisting := []ExistingMemory{{ID: "1", Content: "ユーザーは大岡山に住んでいる"}, {ID: "2", Content: "ユーザーは朝にコーヒーを飲む"}}
	if !reflect.DeepEqual(creator.existing, wantExisting) {
		t.Fatalf("existing = %#v, want %#v", creator.existing, wantExisting)
	}
	update, ok := store.updates["1788926855608507735-3"]
	if !ok || update.Content != "ユーザーは横浜に住んでいる" || !reflect.DeepEqual(update.Tags, []string{"location"}) {
		t.Fatalf("updates = %#v", store.updates)
	}
	if !reflect.DeepEqual(store.deletes, []string{"1789095948492476660-8"}) {
		t.Fatalf("deletes = %#v", store.deletes)
	}
	if len(store.inputs) != 1 || store.inputs[0].Content != "ユーザーは寿司が好き" {
		t.Fatalf("upserts = %#v", store.inputs)
	}
}

func TestCreatorHookRejectsUnknownTargetIDs(t *testing.T) {
	store := &fakeMemoryWriter{records: []memorystate.Record{{ID: "m-1", Content: "既存"}}}
	hook := mustCreatorHook(
		t,
		&fakeHistory{records: []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}}},
		&fakeOperationCreator{operations: []Operation{
			{Action: OperationUpdate, TargetID: "9", Content: "存在しない参照の更新"},
			{Action: OperationDelete, TargetID: "m-1"},
			{Action: OperationDelete, TargetID: "1"},
		}},
		store,
	)
	err := hook.Exec(context.Background())
	if err == nil || !strings.Contains(err.Error(), `"9"`) || !strings.Contains(err.Error(), `"m-1"`) {
		t.Fatalf("Exec() error = %v, want unknown target id errors", err)
	}
	if len(store.updates) != 0 {
		t.Fatalf("updates = %#v, want none", store.updates)
	}
	if !reflect.DeepEqual(store.deletes, []string{"m-1"}) {
		t.Fatalf("deletes = %#v, want [m-1]", store.deletes)
	}
}

func TestCreatorHookAppliesOnlyFirstOperationPerTarget(t *testing.T) {
	tests := []struct {
		name        string
		operations  []Operation
		wantUpdates map[string]memorystate.UpdateInput
		wantDeletes []string
	}{
		{
			name:        "delete then update",
			operations:  []Operation{{Action: OperationDelete, TargetID: "1"}, {Action: OperationUpdate, TargetID: "1", Content: "B"}},
			wantDeletes: []string{"m-1"},
		},
		{
			name:        "update then delete",
			operations:  []Operation{{Action: OperationUpdate, TargetID: "1", Content: "A"}, {Action: OperationDelete, TargetID: "1"}},
			wantUpdates: map[string]memorystate.UpdateInput{"m-1": {Content: "A"}},
		},
		{
			name:        "update twice",
			operations:  []Operation{{Action: OperationUpdate, TargetID: "1", Content: "A"}, {Action: OperationUpdate, TargetID: "1", Content: "B"}},
			wantUpdates: map[string]memorystate.UpdateInput{"m-1": {Content: "A"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeMemoryWriter{records: []memorystate.Record{{ID: "m-1", Content: "既存"}}}
			hook := mustCreatorHook(t, &fakeHistory{records: []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}}}, &fakeOperationCreator{operations: tt.operations}, store)
			err := hook.Exec(context.Background())
			if err == nil || !strings.Contains(err.Error(), "already modified") {
				t.Fatalf("Exec() error = %v, want already modified error", err)
			}
			if !reflect.DeepEqual(store.updates, tt.wantUpdates) {
				t.Fatalf("updates = %#v, want %#v", store.updates, tt.wantUpdates)
			}
			if !reflect.DeepEqual(store.deletes, tt.wantDeletes) {
				t.Fatalf("deletes = %#v, want %#v", store.deletes, tt.wantDeletes)
			}
		})
	}
}

func TestCreatorHookSkipsRecordsChangedWhileCreatingOperations(t *testing.T) {
	base := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	store := &fakeMemoryWriter{records: []memorystate.Record{
		{ID: "m-1", Content: "手動で編集される記憶", UpdatedAt: base},
		{ID: "m-2", Content: "手動で削除される記憶", UpdatedAt: base},
		{ID: "m-3", Content: "変わらない記憶", UpdatedAt: base},
	}}
	creator := &fakeOperationCreator{
		operations: []Operation{
			{Action: OperationUpdate, TargetID: "1", Content: "LLMの更新"},
			{Action: OperationDelete, TargetID: "2"},
			{Action: OperationUpdate, TargetID: "3", Content: "LLMの更新3"},
		},
		// LLM の応答を待つ間に管理画面で編集・削除された状況を再現する。
		onCreate: func() {
			store.records[0].Content = "手動の編集"
			store.records[0].UpdatedAt = base.Add(time.Second)
			store.records = append(store.records[:1], store.records[2:]...)
		},
	}
	hook := mustCreatorHook(t, &fakeHistory{records: []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}}}, creator, store)

	err := hook.Exec(context.Background())
	if err == nil || strings.Count(err.Error(), "changed while creating operations") != 2 {
		t.Fatalf("Exec() error = %v, want 2 conflict errors", err)
	}
	if !reflect.DeepEqual(store.updates, map[string]memorystate.UpdateInput{"m-3": {Content: "LLMの更新3"}}) {
		t.Fatalf("updates = %#v, want only m-3", store.updates)
	}
	if len(store.deletes) != 0 {
		t.Fatalf("deletes = %#v, want none", store.deletes)
	}
}

func TestCreatorHookDeletesInsteadOfUpdatingToDuplicateContent(t *testing.T) {
	store := &fakeMemoryWriter{records: []memorystate.Record{
		{ID: "m-1", Content: "ユーザーは大岡山に住んでいる"},
		{ID: "m-2", Content: "ユーザーは東京に住んでいる"},
	}}
	hook := mustCreatorHook(
		t,
		&fakeHistory{records: []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}}},
		&fakeOperationCreator{operations: []Operation{{Action: OperationUpdate, TargetID: "2", Content: " ユーザーは大岡山に住んでいる "}}},
		store,
	)
	if err := hook.Exec(context.Background()); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if len(store.updates) != 0 || !reflect.DeepEqual(store.deletes, []string{"m-2"}) {
		t.Fatalf("updates = %#v, deletes = %#v, want only delete m-2", store.updates, store.deletes)
	}
}

func TestCreatorHookSkipsEmptyHistoryAndEmptyOperations(t *testing.T) {
	t.Run("empty history", func(t *testing.T) {
		creator := &fakeOperationCreator{}
		hook := mustCreatorHook(t, &fakeHistory{}, creator, &fakeMemoryWriter{})
		if err := hook.Exec(context.Background()); err != nil {
			t.Fatalf("Exec() error = %v", err)
		}
		if creator.called {
			t.Fatal("operation creator was called")
		}
	})

	t.Run("empty operations", func(t *testing.T) {
		store := &fakeMemoryWriter{}
		hook := mustCreatorHook(t, &fakeHistory{records: []types.ConversationRecord{{Role: types.RoleUser, Text: "雑談"}}}, &fakeOperationCreator{}, store)
		if err := hook.Exec(context.Background()); err != nil {
			t.Fatalf("Exec() error = %v", err)
		}
		if len(store.inputs) != 0 {
			t.Fatalf("upsert inputs = %#v, want empty", store.inputs)
		}
	})
}

func TestCreatorHookReturnsOperationCreatorError(t *testing.T) {
	wantErr := errors.New("openai failed")
	hook := mustCreatorHook(
		t,
		&fakeHistory{records: []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}}},
		&fakeOperationCreator{err: wantErr},
		&fakeMemoryWriter{},
	)
	err := hook.Exec(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Exec() error = %v, want %v", err, wantErr)
	}
}

func TestCreatorHookContinuesOperationErrors(t *testing.T) {
	upsertErr := errors.New("upsert failed")
	store := &fakeMemoryWriter{errByContent: map[string]error{"保存で失敗する記憶": upsertErr}}
	hook := mustCreatorHook(
		t,
		&fakeHistory{records: []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}}},
		&fakeOperationCreator{operations: []Operation{
			{Action: OperationAdd, Content: "保存で失敗する記憶"},
			{Action: OperationAdd, Content: "保存できる記憶"},
		}},
		store,
	)

	err := hook.Exec(context.Background())
	if !errors.Is(err, upsertErr) {
		t.Fatalf("Exec() error = %v, want upsert error", err)
	}
	if got, want := len(store.inputs), 2; got != want {
		t.Fatalf("upsert count = %d, want %d", got, want)
	}
	if store.inputs[1].Content != "保存できる記憶" {
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
		{name: "operation creator", cfg: CreatorHookConfig{History: &fakeHistory{}}, want: "operation creator is required"},
		{name: "memory", cfg: CreatorHookConfig{History: &fakeHistory{}, OperationCreator: &fakeOperationCreator{}}, want: "memory is required"},
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

func mustCreatorHook(t *testing.T, history *fakeHistory, creator *fakeOperationCreator, store *fakeMemoryWriter) *CreatorHook {
	t.Helper()
	hook, err := NewCreatorHook(CreatorHookConfig{
		History:          history,
		OperationCreator: creator,
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

type fakeOperationCreator struct {
	onCreate   func()
	called     bool
	records    []types.ConversationRecord
	existing   []ExistingMemory
	operations []Operation
	err        error
}

func (f *fakeOperationCreator) CreateOperations(ctx context.Context, records []types.ConversationRecord, existing []ExistingMemory) ([]Operation, error) {
	f.called = true
	f.records = append([]types.ConversationRecord(nil), records...)
	f.existing = append([]ExistingMemory(nil), existing...)
	if f.onCreate != nil {
		f.onCreate()
	}
	if f.err != nil {
		return nil, f.err
	}
	return append([]Operation(nil), f.operations...), nil
}

type fakeMemoryWriter struct {
	records      []memorystate.Record
	inputs       []memorystate.UpsertInput
	updates      map[string]memorystate.UpdateInput
	deletes      []string
	errByContent map[string]error
}

func (f *fakeMemoryWriter) Snapshot() []memorystate.Record {
	return append([]memorystate.Record(nil), f.records...)
}

func (f *fakeMemoryWriter) Upsert(input memorystate.UpsertInput) (memorystate.Record, memorystate.UpsertResult, error) {
	f.inputs = append(f.inputs, input)
	if err := f.errByContent[input.Content]; err != nil {
		return memorystate.Record{}, memorystate.UpsertResult{}, err
	}
	return memorystate.Record{Content: input.Content, Tags: input.Tags}, memorystate.UpsertResult{Created: true}, nil
}

func (f *fakeMemoryWriter) Update(id string, input memorystate.UpdateInput) (memorystate.Record, error) {
	if f.updates == nil {
		f.updates = map[string]memorystate.UpdateInput{}
	}
	f.updates[id] = input
	for i := range f.records {
		if f.records[i].ID == id {
			f.records[i].Content = input.Content
			f.records[i].Tags = input.Tags
			f.records[i].UpdatedAt = f.records[i].UpdatedAt.Add(time.Second)
		}
	}
	return memorystate.Record{ID: id, Content: input.Content, Tags: input.Tags}, nil
}

func (f *fakeMemoryWriter) Delete(id string) error {
	f.deletes = append(f.deletes, id)
	for i := range f.records {
		if f.records[i].ID == id {
			f.records = append(f.records[:i], f.records[i+1:]...)
			break
		}
	}
	return nil
}

func TestCreatorHookKeepsTagsNormalizedByOperationCreator(t *testing.T) {
	// タグ数の上限は MEMORY_MAX_TAGS を持つ OpenAIClient 側で決まるので、hook で切り詰め直さない。
	tags := []string{"a", "b", "c", "d", "e", "f"}
	store := &fakeMemoryWriter{}
	hook := mustCreatorHook(
		t,
		&fakeHistory{records: []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}}},
		&fakeOperationCreator{operations: []Operation{{Action: OperationAdd, Content: "タグが多い記憶", Tags: tags}}},
		store,
	)
	if err := hook.Exec(context.Background()); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if len(store.inputs) != 1 || !reflect.DeepEqual(store.inputs[0].Tags, tags) {
		t.Fatalf("upserts = %#v, want all 6 tags", store.inputs)
	}
}

func TestCreatorHookAppliesDeletesBeforeUpdatesAndAdds(t *testing.T) {
	// 「1 を 2 と同じ本文に更新」と「2 を削除」が同時に来ても、両方の記憶が失われないようにする。
	store := &fakeMemoryWriter{records: []memorystate.Record{
		{ID: "m-1", Content: "ユーザーは大岡山に住んでいる"},
		{ID: "m-2", Content: "ユーザーは横浜に住んでいる"},
	}}
	hook := mustCreatorHook(
		t,
		&fakeHistory{records: []types.ConversationRecord{{Role: types.RoleUser, Text: "test"}}},
		&fakeOperationCreator{operations: []Operation{
			{Action: OperationAdd, Content: "ユーザーは横浜に住んでいる"},
			{Action: OperationUpdate, TargetID: "1", Content: "ユーザーは横浜に住んでいる"},
			{Action: OperationDelete, TargetID: "2"},
		}},
		store,
	)
	if err := hook.Exec(context.Background()); err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if !reflect.DeepEqual(store.deletes, []string{"m-2"}) {
		t.Fatalf("deletes = %#v, want only m-2", store.deletes)
	}
	if got := store.updates["m-1"].Content; got != "ユーザーは横浜に住んでいる" {
		t.Fatalf("m-1 updated content = %q", got)
	}
	if len(store.records) != 1 || store.records[0].Content != "ユーザーは横浜に住んでいる" {
		t.Fatalf("records = %#v, want one record about 横浜", store.records)
	}
}
