package memory

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestCreatorHookPassesSnapshotToOperationCreator(t *testing.T) {
	history := &fakeHistory{records: []types.ConversationRecord{{ID: "rec-1", Role: types.RoleUser, Text: "朝はコーヒー"}}}
	creator := &fakeOperationCreator{operations: []Operation{{Action: OperationAdd, Content: "ユーザーは朝にコーヒーを飲む"}}}
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
	if input.Content != "ユーザーは朝にコーヒーを飲む" {
		t.Fatalf("upsert input = %#v", input)
	}
}

func TestCreatorHookPassesExistingMemoriesAndAppliesOperations(t *testing.T) {
	creator := &fakeOperationCreator{operations: []Operation{
		{Action: OperationUpdate, TargetID: "1", Content: "ユーザーは横浜に住んでいる"},
		{Action: OperationDelete, TargetID: "2"},
		{Action: OperationAdd, Content: "ユーザーは寿司が好き"},
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
	if !ok || update.Content != "ユーザーは横浜に住んでいる" {
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
		{name: "operation creator", cfg: CreatorHookConfig{}, want: "operation creator is required"},
		{name: "memory", cfg: CreatorHookConfig{OperationCreator: &fakeOperationCreator{}}, want: "memory is required"},
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

// testHookは、固定した履歴（fakeHistory）で ExecRecords を呼ぶためのテスト用の包み。
type testHook struct {
	*CreatorHook
	history *fakeHistory
}

func (h testHook) Exec(ctx context.Context) error {
	return h.ExecRecords(ctx, h.history.records)
}

// 古くなった記憶を見分けられるよう、保存済みメモリの更新日時を日本時間で渡す。
func TestCreatorHookPassesUpdatedAtInJapanTime(t *testing.T) {
	creator := &fakeOperationCreator{}
	store := &fakeMemoryWriter{records: []memorystate.Record{
		{ID: "1", Content: "ユーザーの2026-11-01の天気は晴れ", UpdatedAt: time.Date(2026, 11, 1, 0, 30, 0, 0, time.UTC)},
	}}
	hook := mustCreatorHook(t, &fakeHistory{records: []types.ConversationRecord{{Role: types.RoleUser, Text: "おはよう"}}}, creator, store)
	if err := hook.Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(creator.existing) != 1 || creator.existing[0].UpdatedAt != "2026-11-01T09:30:00+09:00" {
		t.Fatalf("existing = %#v", creator.existing)
	}
}

func mustCreatorHook(t *testing.T, history *fakeHistory, creator *fakeOperationCreator, store *fakeMemoryWriter) testHook {
	t.Helper()
	hook, err := NewCreatorHook(CreatorHookConfig{
		OperationCreator: creator,
		Memory:           store,
	})
	if err != nil {
		t.Fatalf("NewCreatorHook() error = %v", err)
	}
	return testHook{CreatorHook: hook, history: history}
}

// fakeHistoryは、メモリ化のために取り出す会話履歴の代わり。
type fakeHistory struct {
	records []types.ConversationRecord
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
	return memorystate.Record{Content: input.Content}, memorystate.UpsertResult{Created: true}, nil
}

func (f *fakeMemoryWriter) Update(id string, input memorystate.UpdateInput) (memorystate.Record, error) {
	if f.updates == nil {
		f.updates = map[string]memorystate.UpdateInput{}
	}
	f.updates[id] = input
	for i := range f.records {
		if f.records[i].ID == id {
			f.records[i].Content = input.Content
			f.records[i].UpdatedAt = f.records[i].UpdatedAt.Add(time.Second)
		}
	}
	return memorystate.Record{ID: id, Content: input.Content}, nil
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

func newTestHistory(t *testing.T, ids ...string) *conversationhistory.Store {
	t.Helper()
	history, err := conversationhistory.NewStore(filepath.Join(t.TempDir(), "conversation_history.json"))
	if err != nil {
		t.Fatal(err)
	}
	// 1件で予算の半分強を使い、直近の1件だけが文脈に残るようにする。
	for _, id := range ids {
		if err := history.Upsert(types.ConversationRecord{ID: id, Role: types.RoleUser, Text: strings.Repeat("あ", 5000)}); err != nil {
			t.Fatal(err)
		}
	}
	return history
}

func waitHistory(t *testing.T, history *conversationhistory.Store, want ...string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		var got []string
		for _, record := range history.Snapshot() {
			got = append(got, record.ID)
		}
		if slices.Equal(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("history = %v, want %v", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestMemorizeOldExtractsOnlyOverflowedRecordsAndRemovesThemAfterSuccess(t *testing.T) {
	history := newTestHistory(t, "unused", "old1", "old2", "recent")
	entered, release := make(chan struct{}), make(chan struct{})
	creator := &fakeOperationCreator{onCreate: func() { close(entered); <-release }}
	hook, err := NewCreatorHook(CreatorHookConfig{OperationCreator: creator, Memory: &fakeMemoryWriter{}})
	if err != nil {
		t.Fatal(err)
	}
	hook.MemorizeOld(context.Background(), history, map[string]bool{"unused": true})
	<-entered
	if len(creator.records) != 2 || creator.records[0].ID != "old1" || creator.records[1].ID != "old2" {
		close(release)
		t.Fatal(creator.records)
	}
	// 抽出の最中に届いた記録は、完了後の削除で消さない。
	if err := history.Upsert(types.ConversationRecord{ID: "new", Text: "次の会話"}); err != nil {
		t.Fatal(err)
	}
	close(release)
	waitHistory(t, history, "unused", "recent", "new")
}

func TestMemorizeOldKeepsRecordsWhenExtractionFails(t *testing.T) {
	history := newTestHistory(t, "old", "recent")
	done := make(chan struct{})
	creator := &fakeOperationCreator{err: errors.New("抽出失敗"), onCreate: func() { close(done) }}
	hook, err := NewCreatorHook(CreatorHookConfig{OperationCreator: creator, Memory: &fakeMemoryWriter{}})
	if err != nil {
		t.Fatal(err)
	}
	hook.MemorizeOld(context.Background(), history, nil)
	<-done
	// 失敗後に消されないことを、少し待ってから確かめる。
	time.Sleep(50 * time.Millisecond)
	waitHistory(t, history, "old", "recent")
}

// 一部の操作を反映できなくても抽出は済んでいるため、再試行で追加を重複させないよう履歴を消す。
func TestMemorizeOldRemovesRecordsWhenOperationsArePartiallyApplied(t *testing.T) {
	history := newTestHistory(t, "old", "recent")
	creator := &fakeOperationCreator{operations: []Operation{{Action: OperationDelete, TargetID: "9"}}}
	hook, err := NewCreatorHook(CreatorHookConfig{OperationCreator: creator, Memory: &fakeMemoryWriter{}})
	if err != nil {
		t.Fatal(err)
	}
	hook.MemorizeOld(context.Background(), history, nil)
	waitHistory(t, history, "recent")
}

func TestMemorizeOldDoesNothingWhenAllRecordsFitInContext(t *testing.T) {
	history := newTestHistory(t, "recent")
	creator := &fakeOperationCreator{}
	hook, err := NewCreatorHook(CreatorHookConfig{OperationCreator: creator, Memory: &fakeMemoryWriter{}})
	if err != nil {
		t.Fatal(err)
	}
	// 対象がなければゴルーチンを起動せずに同期で返る。
	hook.MemorizeOld(context.Background(), history, nil)
	if creator.called {
		t.Fatal("文脈に入る記録をメモリ化しました")
	}
}
