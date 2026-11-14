package types

// 会話管理へ渡す3種類のイベントと、接続・無操作の通知のためのイベント。
const (
	EventConversationRecord EventKind = 130 + iota
	EventConversationContextAddition
	EventConversationResponseRequest
	EventConversationHistorySync
	// EventConversationIdle は、会話が落ち着いた状態で無操作が続いたことを知らせる。古い履歴のメモリ化に使う。
	EventConversationIdle
	EventConversationRecordsUsed
)

// ConversationInput は保存する記録と、通知表示用の情報を持つ。
// 応答を要求するかはイベントの種類で表し、履歴には保存しない。
type ConversationInput struct {
	Record   ConversationRecord
	Title    string
	External bool
}

// ConversationHistorySync は先行入力の配送と直接読み出しの順序を揃える。
// 履歴本体は運ばず、読了時にDoneを閉じて後続の保存を再開する。
type ConversationHistorySync struct {
	Epoch uint64
	Done  chan struct{}
}

// ConversationRecordsUsed は応答開始時点で参照した外部記録を通知する。
type ConversationRecordsUsed struct{ IDs []string }
