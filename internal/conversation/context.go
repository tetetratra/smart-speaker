package conversation

import (
	"encoding/json"
	"github.com/tetetratra/smart-speaker/internal/types"
	"strings"
)

// TrimHistory は接続復元に渡す履歴を古い記録から除く。
// UTF-8のバイト数を保守的な予算として使う。既定値は他の指示・ツール用の余裕を残す。
// ストアの履歴は変更せず、要約のためのAPIも呼ばない。
func TrimHistory(records []types.ConversationRecord, memory string, maxBytes int) []types.ConversationRecord {
	if maxBytes <= 0 {
		maxBytes = 24000
	}
	parts := make([]string, len(records))
	total := len(memory)
	for i, record := range records {
		raw := contextRecord(record)
		parts[i] = string(raw) + "\n"
		total += len(parts[i])
	}
	start := 0
	for start < len(parts) && total > maxBytes {
		total -= len(parts[start])
		start++
	}
	// 結果だけを残すと対応する呼び出しを復元できないため、孤立した先頭の結果も外す。
	for start < len(records) && records[start].Role == types.RoleToolResult {
		start++
	}
	return records[start:]
}

func RestoreContext(records []types.ConversationRecord, memory string, maxBytes int) string {
	var text strings.Builder
	for _, record := range TrimHistory(records, memory, maxBytes) {
		raw := contextRecord(record)
		text.Write(raw)
		text.WriteByte('\n')
	}
	text.WriteString(memory)
	return text.String()
}

// 音声の未送信部分などの内部メタデータを、話した内容としてモデルへ復元しない。
func contextRecord(record types.ConversationRecord) []byte {
	record.Metadata = nil
	raw, _ := json.Marshal(record)
	return raw
}
