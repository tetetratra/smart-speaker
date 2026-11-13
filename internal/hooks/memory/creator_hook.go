package memory

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/tetetratra/smart-speaker/internal/conversation"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

// errPartiallyApplied は、抽出はできたが一部の操作を反映できなかったことを表す。
// 主な原因は抽出中の手動編集や不明な参照番号で、同じ会話で再試行しても直らず、反映済みの追加が重複するだけになる。
// メモリの保存の失敗（ディスクI/O）も含むが、その記憶はメモリ上には残るため区別しない。
var errPartiallyApplied = errors.New("memory operations partially applied")

type CreatorHookConfig struct {
	OperationCreator operationCreator
	Memory           memoryWriter
}

type CreatorHook struct {
	operationCreator operationCreator
	memory           memoryWriter
}

type operationCreator interface {
	CreateOperations(context.Context, []types.ConversationRecord, []ExistingMemory) ([]Operation, error)
}

type memoryWriter interface {
	Snapshot() []memorystate.Record
	Upsert(memorystate.UpsertInput) (memorystate.Record, memorystate.UpsertResult, error)
	Update(string, memorystate.UpdateInput) (memorystate.Record, error)
	Delete(string) error
}

func NewCreatorHook(cfg CreatorHookConfig) (*CreatorHook, error) {
	if cfg.OperationCreator == nil {
		return nil, fmt.Errorf("memory creator hook: operation creator is required")
	}
	if cfg.Memory == nil {
		return nil, fmt.Errorf("memory creator hook: memory is required")
	}
	return &CreatorHook{
		operationCreator: cfg.OperationCreator,
		memory:           cfg.Memory,
	}, nil
}

// MemorizeOld は、接続時に復元する文脈からあふれた古い記録を長期メモリにする。
// 文脈に入る範囲は接続時の復元と同じ予算で決め、まだ応答に使っていない外部記録（keep）は対象にしない。
// 抽出に成功した記録だけを履歴から消し、失敗したら次の発火で再試行できるよう残す。
func (h *CreatorHook) MemorizeOld(ctx context.Context, history *conversationhistory.Store, keep map[string]bool) {
	messages, err := memoryContextMessages(h.memory.Snapshot())
	if err != nil {
		log.Printf("古い履歴のメモリ化: 長期メモリを読めないため見送ります: %v", err)
		return
	}
	memory := ""
	for _, m := range messages {
		memory += m.Content + "\n"
	}
	var records []types.ConversationRecord
	for _, record := range history.Snapshot() {
		if !keep[record.ID] {
			records = append(records, record)
		}
	}
	old := records[:len(records)-len(conversation.TrimHistory(records, memory, 0))]
	if len(old) == 0 {
		return
	}
	ids := make([]string, len(old))
	for i, record := range old {
		ids[i] = record.ID
	}
	log.Printf("古い履歴のメモリ化を開始します: %d件", len(old))
	go func() {
		err := h.ExecRecords(ctx, old)
		if errors.Is(err, errPartiallyApplied) {
			// 抽出自体は済んでいるため、再試行で追加を重複させないよう履歴は消す。
			log.Printf("古い履歴のメモリ化: %v", err)
		} else if err != nil {
			log.Printf("古い履歴のメモリ化に失敗しました。履歴は残します: %v", err)
			return
		}
		if err := history.Remove(ids); err != nil {
			log.Printf("古い履歴のメモリ化: 履歴の削除: %v", err)
			return
		}
		log.Printf("古い履歴のメモリ化が完了しました: %d件", len(old))
	}()
}

// ExecRecords は取り出した時点で固定した記録を対象にする。
// 保存済みメモリを一緒に渡し、LLM が返した add / update / delete を順に適用する。
func (h *CreatorHook) ExecRecords(ctx context.Context, records []types.ConversationRecord) error {
	if len(records) == 0 {
		return nil
	}
	snapshot := h.memory.Snapshot()
	// store の ID は長い数値なので、LLM が書き写し間違えないよう短い参照番号に置き換えて渡す。
	existing := make([]ExistingMemory, 0, len(snapshot))
	byRef := make(map[string]memorystate.Record, len(snapshot))
	for i, record := range snapshot {
		ref := strconv.Itoa(i + 1)
		item := ExistingMemory{ID: ref, Content: record.Content}
		if !record.UpdatedAt.IsZero() {
			item.UpdatedAt = record.UpdatedAt.In(japanLocation).Format(time.RFC3339)
		}
		existing = append(existing, item)
		byRef[ref] = record
	}
	operations, err := h.operationCreator.CreateOperations(ctx, records, existing)
	if err != nil {
		return err
	}
	var errs []error
	var adds, updates, deletes []resolvedOperation
	targeted := map[string]struct{}{}
	for _, operation := range operations {
		if operation.Action == OperationAdd {
			adds = append(adds, resolvedOperation{Operation: operation})
			continue
		}
		// 存在しない参照番号は、誤って別の記憶を変えないよう適用しない。
		original, ok := byRef[operation.TargetID]
		if !ok {
			errs = append(errs, fmt.Errorf("%s memory: unknown target id %q", operation.Action, operation.TargetID))
			continue
		}
		// 同じ記憶への2件目以降の操作は、先の操作と矛盾しうるので捨てる。
		if _, done := targeted[operation.TargetID]; done {
			errs = append(errs, fmt.Errorf("%s memory %q: already modified in this run", operation.Action, original.ID))
			continue
		}
		targeted[operation.TargetID] = struct{}{}
		resolved := resolvedOperation{Operation: operation, original: original}
		switch operation.Action {
		case OperationDelete:
			deletes = append(deletes, resolved)
		case OperationUpdate:
			updates = append(updates, resolved)
		default:
			errs = append(errs, fmt.Errorf("memory operation: unknown action %q", operation.Action))
		}
	}
	// delete を先に適用する。update / add の重複判定が、この後消える記憶を根拠にして両方を失うのを防ぐ。
	for _, resolved := range deletes {
		if err := h.modify(resolved); err != nil {
			errs = append(errs, err)
		}
	}
	for _, resolved := range updates {
		if err := h.modify(resolved); err != nil {
			errs = append(errs, err)
		}
	}
	for _, resolved := range adds {
		if err := h.add(resolved.Operation); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", errPartiallyApplied, errors.Join(errs...))
	}
	return nil
}

// resolvedOperation は参照番号を抽出開始時点の記憶に解決した操作。
type resolvedOperation struct {
	Operation
	original memorystate.Record
}

func (h *CreatorHook) add(operation Operation) error {
	if _, _, err := h.memory.Upsert(memorystate.UpsertInput{Content: operation.Content}); err != nil {
		return fmt.Errorf("add memory %q: %w", operation.Content, err)
	}
	return nil
}

// modify は抽出開始時点の original を前提にした update / delete を適用する。
func (h *CreatorHook) modify(resolved resolvedOperation) error {
	original, operation := resolved.original, resolved.Operation
	current, others := splitRecord(h.memory.Snapshot(), original.ID)
	// LLM の応答を待つ間に管理画面から編集・削除されていたら、手動の変更を優先して何もしない。
	// 確認から書き込みまでの間はロックの外だが、その間隔は短く、LLM 待ちの数秒の競合を防げれば十分とする。
	if current == nil || !current.UpdatedAt.Equal(original.UpdatedAt) {
		return fmt.Errorf("%s memory %q: changed while creating operations", operation.Action, original.ID)
	}
	action := operation.Action
	// 更新後の本文が別の記憶と同じなら、重複を作らないよう対象を消すだけにする。
	if action == OperationUpdate && hasSameContent(others, operation.Content) {
		action = OperationDelete
	}
	if action == OperationDelete {
		if err := h.memory.Delete(original.ID); err != nil {
			return fmt.Errorf("delete memory %q: %w", original.ID, err)
		}
		return nil
	}
	if _, err := h.memory.Update(original.ID, memorystate.UpdateInput{Content: operation.Content}); err != nil {
		return fmt.Errorf("update memory %q: %w", original.ID, err)
	}
	return nil
}

func splitRecord(records []memorystate.Record, id string) (*memorystate.Record, []memorystate.Record) {
	var target *memorystate.Record
	others := make([]memorystate.Record, 0, len(records))
	for i := range records {
		if records[i].ID == id {
			target = &records[i]
			continue
		}
		others = append(others, records[i])
	}
	return target, others
}

func hasSameContent(records []memorystate.Record, content string) bool {
	for _, record := range records {
		if memorystate.SameContent(record.Content, content) {
			return true
		}
	}
	return false
}
