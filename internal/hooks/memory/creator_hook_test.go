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

	"github.com/tetetratra/smart-speaker/internal/conversation"
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

	applied, err := hook.ExecRecords(context.Background(), hook.history.records)
	if err != nil {
		t.Fatalf("ExecRecords() error = %v", err)
	}
	if want := (appliedCounts{added: 1, updated: 1, deleted: 1}); applied != want {
		t.Fatalf("applied = %+v, want %+v", applied, want)
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
	applied, err := hook.ExecRecords(context.Background(), hook.history.records)
	if err != nil {
		t.Fatalf("ExecRecords() error = %v", err)
	}
	// 実際に行ったのは削除なので、削除として数える。
	if want := (appliedCounts{deleted: 1}); applied != want {
		t.Fatalf("applied = %+v, want %+v", applied, want)
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
		{name: "deliver", cfg: CreatorHookConfig{OperationCreator: &fakeOperationCreator{}, Memory: &fakeMemoryWriter{}}, want: "deliver is required"},
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
	_, err := h.ExecRecords(ctx, h.history.records)
	return err
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
		Deliver:          func(context.Context, types.ConversationInput) error { return nil },
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

// testNow は、メモリ化のテストで「今」とする日時。
var testNow = time.Date(2026, 11, 15, 12, 0, 0, 0, japanLocation)

// at は、testNow から ago だけ前に作られた記録を作る。
func at(id string, ago time.Duration) types.ConversationRecord {
	return types.ConversationRecord{ID: id, Role: types.RoleUser, Text: "話した内容", CreatedAt: testNow.Add(-ago)}
}

func newTestHistory(t *testing.T, records ...types.ConversationRecord) *conversationhistory.Store {
	t.Helper()
	history, err := conversationhistory.NewStore(filepath.Join(t.TempDir(), "conversation_history.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if err := history.Append(record); err != nil {
			t.Fatal(err)
		}
	}
	return history
}

// memorizeHook は、メモリ化したことを知らせる記録の送り先（notices）をテストから読めるようにした包み。
type memorizeHook struct {
	*CreatorHook
	notices chan types.ConversationInput
}

func newMemorizeHook(t *testing.T, creator *fakeOperationCreator, maxHistoryBytes int) memorizeHook {
	t.Helper()
	notices := make(chan types.ConversationInput, 1)
	hook, err := NewCreatorHook(CreatorHookConfig{
		OperationCreator: creator,
		Memory:           &fakeMemoryWriter{},
		Deliver: func(_ context.Context, input types.ConversationInput) error {
			notices <- input
			return nil
		},
		MaxHistoryBytes: maxHistoryBytes,
		Now:             func() time.Time { return testNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	return memorizeHook{CreatorHook: hook, notices: notices}
}

func historyIDs(history *conversationhistory.Store) []string {
	var ids []string
	for _, record := range history.Snapshot() {
		ids = append(ids, record.ID)
	}
	return ids
}

func waitHistory(t *testing.T, history *conversationhistory.Store, want ...string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		got := historyIDs(history)
		if slices.Equal(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("history = %v, want %v", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// waitNotice は、メモリ化が終わって知らせの記録が届くまで待つ。知らせは履歴から消したあとに届く。
func (h memorizeHook) waitNotice(t *testing.T) types.ConversationInput {
	t.Helper()
	select {
	case input := <-h.notices:
		return input
	case <-time.After(time.Second):
		t.Fatal("メモリ化したことを知らせる記録が届きません")
		return types.ConversationInput{}
	}
}

func recordIDs(records []types.ConversationRecord) []string {
	var ids []string
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return ids
}

// 最後の記録から24時間たった塊だけを丸ごとメモリ化し、24時間の境目をまたぐ塊は残す。
func TestMemorizeOldExtractsChunksOlderThanMemorizeAfter(t *testing.T) {
	history := newTestHistory(t,
		at("a1", 50*time.Hour), at("a2", 49*time.Hour), // 塊A: 24時間より前
		at("b1", 24*time.Hour+10*time.Minute), at("b2", 23*time.Hour+50*time.Minute), // 塊B: 境目をまたぐ
		at("c1", time.Hour),
	)
	done := make(chan struct{})
	creator := &fakeOperationCreator{onCreate: func() { close(done) }}
	creator.operations = []Operation{{Action: OperationAdd, Content: "ユーザーは寿司が好き"}, {Action: OperationAdd, Content: "ユーザーは猫を飼っている"}}
	hook := newMemorizeHook(t, creator, 0)
	hook.MemorizeOld(context.Background(), history, nil)
	<-done
	if got := recordIDs(creator.records); !slices.Equal(got, []string{"a1", "a2"}) {
		t.Fatalf("extracted = %v, want [a1 a2]", got)
	}
	// 履歴から消したあと、メモリ化したことを会話へ届ける。モデルには伝えず、画面にだけ出す。
	input := hook.waitNotice(t)
	if ids := historyIDs(history); !slices.Equal(ids, []string{"b1", "b2", "c1"}) {
		t.Fatalf("history = %v", ids)
	}
	if !input.ModelAlreadyKnows || input.UserAlreadyKnows || input.ModelShouldRespond || input.Subscribable {
		t.Fatalf("input = %#v", input)
	}
	notice := input.Record
	if !strings.HasPrefix(notice.ID, "memorize/") || notice.Role != types.RoleSystem || notice.Source != NoticeSource || !notice.CreatedAt.Equal(testNow) {
		t.Fatalf("notice = %#v", notice)
	}
	if want := "11月13日 11:00 までの会話を長期メモリにしました（追加2件・更新0件・削除0件）。"; notice.Text != want {
		t.Fatalf("notice text = %q, want %q", notice.Text, want)
	}
}

// 24時間たっていなくても、履歴がしきい値を超えるなら古い塊から足していき、超えなくなったら止める。
func TestMemorizeOldExtractsNewerChunksWhileHistoryIsTooLarge(t *testing.T) {
	history := newTestHistory(t,
		at("a1", 5*time.Hour), at("a2", 5*time.Hour-time.Minute),
		at("b1", 3*time.Hour),
		at("c1", time.Hour),
	)
	size := conversation.HistoryBytes(history.Snapshot()[2:])
	done := make(chan struct{})
	creator := &fakeOperationCreator{onCreate: func() { close(done) }}
	// 塊Aを除けば収まる大きさにする。
	hook := newMemorizeHook(t, creator, size)
	hook.MemorizeOld(context.Background(), history, nil)
	<-done
	if got := recordIDs(creator.records); !slices.Equal(got, []string{"a1", "a2"}) {
		t.Fatalf("extracted = %v, want [a1 a2]", got)
	}
	hook.waitNotice(t)
}

// 区切りがなく、最新の塊までしきい値を超えるなら、残りを全部メモリ化する。
func TestMemorizeOldExtractsLastChunkWhenItAloneIsTooLarge(t *testing.T) {
	history := newTestHistory(t, at("a1", 2*time.Hour), at("a2", time.Hour), at("a3", 31*time.Minute))
	done := make(chan struct{})
	creator := &fakeOperationCreator{onCreate: func() { close(done) }}
	hook := newMemorizeHook(t, creator, 1)
	hook.MemorizeOld(context.Background(), history, nil)
	<-done
	if got := recordIDs(creator.records); !slices.Equal(got, []string{"a1", "a2", "a3"}) {
		t.Fatalf("extracted = %v, want all", got)
	}
	hook.waitNotice(t)
}

// まだ応答に使っていない外部記録は対象にしない。抽出の最中に届いた記録も、完了後の削除で消さない。
func TestMemorizeOldSkipsUnusedRecordsAndKeepsRecordsAddedDuringExtraction(t *testing.T) {
	history := newTestHistory(t, at("unused", 49*time.Hour), at("old", 48*time.Hour), at("recent", time.Hour))
	entered, release := make(chan struct{}), make(chan struct{})
	creator := &fakeOperationCreator{onCreate: func() { close(entered); <-release }}
	hook := newMemorizeHook(t, creator, 0)
	hook.MemorizeOld(context.Background(), history, map[string]bool{"unused": true})
	<-entered
	if got := recordIDs(creator.records); !slices.Equal(got, []string{"old"}) {
		close(release)
		t.Fatalf("extracted = %v, want [old]", got)
	}
	if err := history.Append(types.ConversationRecord{ID: "new", Text: "次の会話"}); err != nil {
		t.Fatal(err)
	}
	close(release)
	hook.waitNotice(t)
	if got := historyIDs(history); !slices.Equal(got, []string{"unused", "recent", "new"}) {
		t.Fatalf("history = %v", got)
	}
}

// メモリ化したことを知らせる記録は、それ自体をメモリにしない。古くなったら消すだけで、新しい知らせも足さない。
func TestMemorizeOldRemovesOldNoticesWithoutExtractingThem(t *testing.T) {
	notice := at("memorize/1", 30*time.Hour)
	notice.Role, notice.Source = types.RoleSystem, NoticeSource
	history := newTestHistory(t, notice, at("recent", time.Hour))
	creator := &fakeOperationCreator{}
	hook := newMemorizeHook(t, creator, 0)
	hook.MemorizeOld(context.Background(), history, nil)
	waitHistory(t, history, "recent")
	time.Sleep(50 * time.Millisecond)
	if creator.called {
		t.Fatal("知らせの記録を抽出に渡しました")
	}
	if len(hook.notices) > 0 {
		t.Fatal("知らせだけを消したのに、新しい知らせを届けました")
	}
}

func TestMemorizeOldKeepsRecordsWhenExtractionFails(t *testing.T) {
	history := newTestHistory(t, at("old", 48*time.Hour), at("recent", time.Hour))
	done := make(chan struct{})
	creator := &fakeOperationCreator{err: errors.New("抽出失敗"), onCreate: func() { close(done) }}
	newMemorizeHook(t, creator, 0).MemorizeOld(context.Background(), history, nil)
	<-done
	// 失敗後に消されないことを、少し待ってから確かめる。
	time.Sleep(50 * time.Millisecond)
	waitHistory(t, history, "old", "recent")
}

// 一部の操作を反映できなくても抽出は済んでいるため、再試行で追加を重複させないよう履歴を消す。
func TestMemorizeOldRemovesRecordsWhenOperationsArePartiallyApplied(t *testing.T) {
	history := newTestHistory(t, at("old", 48*time.Hour), at("recent", time.Hour))
	creator := &fakeOperationCreator{operations: []Operation{{Action: OperationDelete, TargetID: "9"}, {Action: OperationAdd, Content: "ユーザーは寿司が好き"}}}
	hook := newMemorizeHook(t, creator, 0)
	hook.MemorizeOld(context.Background(), history, nil)
	// 件数は反映できた操作だけを数える。
	if got, want := hook.waitNotice(t).Record.Text, "（追加1件・更新0件・削除0件）"; !strings.Contains(got, want) {
		t.Fatalf("notice text = %q, want it to contain %q", got, want)
	}
	if got := historyIDs(history); !slices.Equal(got, []string{"recent"}) {
		t.Fatalf("history = %v", got)
	}
}

func TestMemorizeOldDoesNothingWhenRecordsAreRecentAndSmall(t *testing.T) {
	history := newTestHistory(t, at("a", 23*time.Hour), at("b", time.Hour))
	creator := &fakeOperationCreator{}
	// 対象がなければゴルーチンを起動せずに同期で返る。
	newMemorizeHook(t, creator, 0).MemorizeOld(context.Background(), history, nil)
	if creator.called {
		t.Fatal("新しい記録をメモリ化しました")
	}
}

func TestSplitChunksSplitsAtGapsOfThirtyMinutes(t *testing.T) {
	records := []types.ConversationRecord{at("a1", 3*time.Hour), at("a2", 2*time.Hour+31*time.Minute), at("b1", 2*time.Hour), at("b2", time.Hour+31*time.Minute)}
	chunks := splitChunks(records)
	if len(chunks) != 2 || !slices.Equal(recordIDs(chunks[0]), []string{"a1", "a2"}) || !slices.Equal(recordIDs(chunks[1]), []string{"b1", "b2"}) {
		t.Fatalf("chunks = %v", chunks)
	}
}
