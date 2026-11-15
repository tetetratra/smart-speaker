package conversation

import (
	"encoding/json"
	"strings"

	"github.com/tetetratra/smart-speaker/internal/types"
)

// HistoryBytes は、記録をモデルに渡す形にしたときの UTF-8 のバイト数を返す。
// 古い履歴をメモリ化するかどうかの判定に使う。
func HistoryBytes(records []types.ConversationRecord) int {
	total := 0
	for _, record := range records {
		total += len(contextRecord(record)) + 1
	}
	return total
}

// RestoreContext は接続時に渡す文脈を作る。履歴はメモリ化で一定の大きさに保つので、ここでは切り詰めない。
func RestoreContext(records []types.ConversationRecord, memory string) string {
	var text strings.Builder
	for _, record := range records {
		text.Write(contextRecord(record))
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
