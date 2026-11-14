package types

// 会話管理とやりとりするイベント。会話の中身は EventConversationInput で運び、
// 接続・無操作・応答への利用を知らせる進行の合図は EventConversationSignal で運ぶ。
// 合図の種類はペイロードの型（ConversationHistorySync・ConversationIdle・ConversationRecordsUsed）で分ける。
const (
	EventConversationInput EventKind = 130 + iota
	EventConversationSignal
	// EventConversationRecordChanged は、会話管理が保存した記録を画面へ知らせる。ペイロードは ConversationRecord。
	// 世代を持たない型なので、世代フィルタに捨てられない（EventGeneration に加えないこと）。
	EventConversationRecordChanged
)

// ModelHandling は、会話の中身をモデルに対してどう扱うかを表す。
type ModelHandling string

const (
	ModelHandlingAlreadyKnown  ModelHandling = "already_known"   // モデルはもう知っている。記録だけする
	ModelHandlingAddToContext  ModelHandling = "add_to_context"  // モデルに足すが、応答はさせない
	ModelHandlingAddAndRespond ModelHandling = "add_and_respond" // モデルに足し、応答も依頼する
)

// ConversationInput は保存する記録と、モデルへの扱いを持つ。
// モデルへの扱いは処理の分岐にだけ使い、履歴には保存しない。
type ConversationInput struct {
	Record        ConversationRecord
	ModelHandling ModelHandling
}

// ConversationHistorySync は先行入力の配送と直接読み出しの順序を揃える。
// 履歴本体は運ばず、読了時にDoneを閉じて後続の保存を再開する。
type ConversationHistorySync struct {
	Epoch uint64
	Done  chan struct{}
}

// ConversationIdle は、会話が落ち着いた状態で無操作が続いたことを知らせる。古い履歴のメモリ化に使う。
type ConversationIdle struct{}

// ConversationRecordsUsed は応答開始時点で参照した外部記録を通知する。
type ConversationRecordsUsed struct{ IDs []string }
