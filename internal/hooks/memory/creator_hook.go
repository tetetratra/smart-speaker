package memory

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/components/gateway"
	"github.com/tetetratra/smart-speaker/internal/conversation"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

// errPartiallyApplied は、抽出はできたが一部の操作を反映できなかったことを表す。
// 主な原因は抽出中の手動編集や不明な参照番号で、同じ会話で再試行しても直らず、反映済みの追加が重複するだけになる。
// メモリの保存の失敗（ディスクI/O）も含むが、その記憶はメモリ上には残るため区別しない。
var errPartiallyApplied = errors.New("memory operations partially applied")

const (
	// DefaultMemorizeAfter は、会話をメモリ化するまでの既定の経過時間。
	DefaultMemorizeAfter = 24 * time.Hour
	// DefaultMaxHistoryBytes は、経過時間によらずメモリ化を始める履歴の大きさの既定値。
	DefaultMaxHistoryBytes = 40000
	// chunkGap は、会話の塊を区切る記録の間隔。メモリ化を始める無操作時間の既定値と同じにする。
	chunkGap = 30 * time.Minute
	// NoticeSource は、メモリ化したことを履歴に残す記録の Source。
	NoticeSource = "memorize"
)

type CreatorHookConfig struct {
	OperationCreator operationCreator
	Memory           memoryWriter
	// Deliver は、メモリ化したことを知らせる記録を会話へ届ける。会話管理が保存してホワイトボードに出す。
	Deliver gateway.Deliver
	// MemorizeAfter は、最後の記録からこの時間がたった塊をメモリ化する。0 以下なら既定値。
	MemorizeAfter time.Duration
	// MaxHistoryBytes は、履歴がこれを超えたら新しい塊もメモリ化する。0 以下なら既定値。
	MaxHistoryBytes int
	// Now は今の日時。テストで差し替える。
	Now func() time.Time
}

type CreatorHook struct {
	operationCreator operationCreator
	memory           memoryWriter
	deliver          gateway.Deliver
	memorizeAfter    time.Duration
	maxHistoryBytes  int
	now              func() time.Time
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
	if cfg.Deliver == nil {
		return nil, fmt.Errorf("memory creator hook: deliver is required")
	}
	h := &CreatorHook{
		operationCreator: cfg.OperationCreator,
		memory:           cfg.Memory,
		deliver:          cfg.Deliver,
		memorizeAfter:    cfg.MemorizeAfter,
		maxHistoryBytes:  cfg.MaxHistoryBytes,
		now:              cfg.Now,
	}
	if h.memorizeAfter <= 0 {
		h.memorizeAfter = DefaultMemorizeAfter
	}
	if h.maxHistoryBytes <= 0 {
		h.maxHistoryBytes = DefaultMaxHistoryBytes
	}
	if h.now == nil {
		h.now = time.Now
	}
	return h, nil
}

// MemorizeOld は、古い会話を塊の単位で長期メモリにする。
// まだ応答に使っていない外部記録（keep）は対象にしない。
// 抽出に成功した記録だけを履歴から消し、失敗したら次の発火で再試行できるよう残す。
func (h *CreatorHook) MemorizeOld(ctx context.Context, history *conversationhistory.Store, keep map[string]bool) {
	var records []types.ConversationRecord
	for _, record := range history.Snapshot() {
		if !keep[record.ID] {
			records = append(records, record)
		}
	}
	old := h.selectOld(records)
	if len(old) == 0 {
		return
	}
	ids := make([]string, len(old))
	// メモリ化したことを知らせる記録は、それ自体をメモリにしない。時間がたてば履歴から消すだけにする。
	var targets []types.ConversationRecord
	for i, record := range old {
		ids[i] = record.ID
		if record.Source != NoticeSource {
			targets = append(targets, record)
		}
	}
	until := old[len(old)-1].CreatedAt
	log.Printf("古い履歴のメモリ化を開始します: %d件", len(old))
	go func() {
		applied, err := h.ExecRecords(ctx, targets)
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
		if len(targets) > 0 {
			// モデルには伝えない。接続中のセッションはメモリ化した会話をまだ持っていて、
			// この記録が要るのは古い会話が見えなくなる次の接続からで、そのときは履歴から渡る。
			input := types.ConversationInput{Record: h.notice(until, applied), ModelAlreadyKnows: true}
			if err := h.deliver(ctx, input); err != nil {
				log.Printf("古い履歴のメモリ化: メモリ化した記録の送信: %v", err)
			}
		}
		log.Printf("古い履歴のメモリ化が完了しました: %d件", len(old))
	}()
}

// selectOld は、メモリ化する記録を古いほうから塊の単位で選ぶ。
// 最後の記録から memorizeAfter がたった塊を選び、それでも残りが maxHistoryBytes を超えるなら、超えなくなるまで次の塊も選ぶ。
func (h *CreatorHook) selectOld(records []types.ConversationRecord) []types.ConversationRecord {
	chunks := splitChunks(records)
	deadline := h.now().Add(-h.memorizeAfter)
	n := 0
	for n < len(chunks) && !chunks[n][len(chunks[n])-1].CreatedAt.After(deadline) {
		n++
	}
	selected := 0
	for _, chunk := range chunks[:n] {
		selected += len(chunk)
	}
	for n < len(chunks) && conversation.HistoryBytes(records[selected:]) > h.maxHistoryBytes {
		selected += len(chunks[n])
		n++
	}
	return records[:selected]
}

// splitChunks は、記録の間隔が chunkGap 以上空いた位置で履歴を区切る。最後の塊は最新の記録まで。
func splitChunks(records []types.ConversationRecord) [][]types.ConversationRecord {
	var chunks [][]types.ConversationRecord
	start := 0
	for i := 1; i < len(records); i++ {
		if records[i].CreatedAt.Sub(records[i-1].CreatedAt) >= chunkGap {
			chunks = append(chunks, records[start:i])
			start = i
		}
	}
	if start < len(records) {
		chunks = append(chunks, records[start:])
	}
	return chunks
}

// notice は、until までの会話をメモリ化したことと、反映できた操作の件数を知らせる記録を作る。
func (h *CreatorHook) notice(until time.Time, applied appliedCounts) types.ConversationRecord {
	return types.ConversationRecord{
		ID:     "memorize/" + uuid.NewString(),
		Role:   types.RoleSystem,
		Source: NoticeSource,
		Text: fmt.Sprintf("%s までの会話を長期メモリにしました（追加%d件・更新%d件・削除%d件）。",
			until.In(japanLocation).Format("1月2日 15:04"), applied.added, applied.updated, applied.deleted),
		CreatedAt: h.now(),
	}
}

// appliedCounts は、メモリに反映できた操作の件数。
type appliedCounts struct{ added, updated, deleted int }

// ExecRecords は取り出した時点で固定した記録を対象にする。
// 保存済みメモリを一緒に渡し、LLM が返した add / update / delete を順に適用して、反映できた件数を返す。
func (h *CreatorHook) ExecRecords(ctx context.Context, records []types.ConversationRecord) (appliedCounts, error) {
	var applied appliedCounts
	if len(records) == 0 {
		return applied, nil
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
		return applied, err
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
	// update は重複を避けて delete になることがあるので、件数は実際に行った操作で数える。
	for _, resolved := range append(deletes, updates...) {
		action, err := h.modify(resolved)
		if err != nil {
			errs = append(errs, err)
		} else if action == OperationDelete {
			applied.deleted++
		} else {
			applied.updated++
		}
	}
	for _, resolved := range adds {
		if err := h.add(resolved.Operation); err != nil {
			errs = append(errs, err)
		} else {
			applied.added++
		}
	}
	if len(errs) > 0 {
		return applied, fmt.Errorf("%w: %w", errPartiallyApplied, errors.Join(errs...))
	}
	return applied, nil
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

// modify は抽出開始時点の original を前提にした update / delete を適用し、実際に行った操作を返す。
func (h *CreatorHook) modify(resolved resolvedOperation) (string, error) {
	original, operation := resolved.original, resolved.Operation
	current, others := splitRecord(h.memory.Snapshot(), original.ID)
	// LLM の応答を待つ間に管理画面から編集・削除されていたら、手動の変更を優先して何もしない。
	// 確認から書き込みまでの間はロックの外だが、その間隔は短く、LLM 待ちの数秒の競合を防げれば十分とする。
	if current == nil || !current.UpdatedAt.Equal(original.UpdatedAt) {
		return "", fmt.Errorf("%s memory %q: changed while creating operations", operation.Action, original.ID)
	}
	action := operation.Action
	// 更新後の本文が別の記憶と同じなら、重複を作らないよう対象を消すだけにする。
	if action == OperationUpdate && hasSameContent(others, operation.Content) {
		action = OperationDelete
	}
	if action == OperationDelete {
		if err := h.memory.Delete(original.ID); err != nil {
			return "", fmt.Errorf("delete memory %q: %w", original.ID, err)
		}
		return action, nil
	}
	if _, err := h.memory.Update(original.ID, memorystate.UpdateInput{Content: operation.Content}); err != nil {
		return "", fmt.Errorf("update memory %q: %w", original.ID, err)
	}
	return action, nil
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
