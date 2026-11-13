package types

// 会話管理へ渡す3種類のイベントと、接続・会話終了のためのイベント。
const (
	EventConversationRecord EventKind = 130 + iota
	EventConversationContextAddition
	EventConversationResponseRequest
	EventConversationSnapshotRequest
	EventConversationSnapshot
	EventConversationEnded
	EventConversationRecordsUsed
)

// ConversationInput は保存する記録と、通知表示用の情報を持つ。
// 応答を要求するかはイベントの種類で表し、履歴には保存しない。
type ConversationInput struct {
	Record   ConversationRecord
	Title    string
	External bool
}

// ConversationSnapshotRequest は接続世代に対応する履歴の取得要求。
type ConversationSnapshotRequest struct{ Epoch uint64 }

// ConversationSnapshot は会話管理から返す履歴と長期メモリ。
type ConversationSnapshot struct {
	Epoch   uint64
	Records []ConversationRecord
	Memory  string
}

// ConversationRecordsUsed は応答開始時点で参照した外部記録を通知する。
type ConversationRecordsUsed struct{ IDs []string }
