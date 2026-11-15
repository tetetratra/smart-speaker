package types

// 会話管理とやりとりするイベント。会話の中身は EventConversationInput で運び、
// 接続・無操作・応答への利用を知らせる進行の合図は、合図ごとのイベントで運ぶ。

// ConversationInput は保存する記録と、モデルとユーザーがその記録をもう知っているかを持つ。
// 3つの bool は処理の分岐にだけ使い、履歴には保存しない。
type ConversationInput struct {
	Record ConversationRecord
	// ModelAlreadyKnows は、モデルがもう知っている（文脈に足さず、記録だけする）ことを表す。
	ModelAlreadyKnows bool
	// ModelShouldRespond は、モデルに文脈へ足したうえで応答も依頼することを表す。
	// ModelAlreadyKnows が true のときは使わない。
	ModelShouldRespond bool
	// UserAlreadyKnows は、ユーザーがもう知っている（画面に出さない）ことを表す。
	UserAlreadyKnows bool
}

// EventConversationInput は会話の中身を運ぶ。
type EventConversationInput struct{ ConversationInput }

// EventConversationHistorySync は先行入力の配送と直接読み出しの順序を揃える。
// 履歴本体は運ばず、読了時にDoneを閉じて後続の保存を再開する。
type EventConversationHistorySync struct {
	Epoch uint64
	Done  chan struct{}
}

// EventConversationIdle は、会話が落ち着いた状態で無操作が続いたことを知らせる。古い履歴のメモリ化に使う。
type EventConversationIdle struct{}

// EventConversationRecordsUsed は応答開始時点で参照した外部記録を通知する。
type EventConversationRecordsUsed struct{ IDs []string }

// EventConversationRecordAdded は、会話管理が保存した記録を画面へ知らせる。
// 世代を持たないので、世代フィルタに捨てられない（EventGeneration に加えないこと）。
type EventConversationRecordAdded struct{ ConversationRecord }

func (EventConversationInput) isEvent()       {}
func (EventConversationHistorySync) isEvent() {}
func (EventConversationIdle) isEvent()        {}
func (EventConversationRecordsUsed) isEvent() {}
func (EventConversationRecordAdded) isEvent() {}
