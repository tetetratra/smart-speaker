package conversation

import (
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

// historyLegend は、過去の会話の各行の読み方。ロールの意味はモデルに伝えないと区別しにくい。
const historyLegend = `各行は「[日時] ロール(出どころ): 本文」の形です。日時は日本時間です。
- user: ユーザーの発話・入力
- agent: あなた自身の発話
- tool: あなたが実行したツール1回分（ツール名: 引数 → 結果）
- background: 頼んだ処理（Web検索・Claude Code など）から、あとで届いた結果や知らせ
- system: アプリからの知らせ（予約ジョブ、発話が遮られた記録、居場所の変化など）
`

// RestoreContext は接続時に渡す文脈を作る。履歴はメモリ化で一定の大きさに保つので、ここでは切り詰めない。
// 変わりにくい長期メモリを先に、末尾に増えていく会話を後に置き、接続をまたいでも先頭がそろうようにする。
func RestoreContext(records []types.ConversationRecord, memories []string) string {
	var text strings.Builder
	if len(memories) > 0 {
		text.WriteString("## 長期メモリ\n")
		for _, memory := range memories {
			text.WriteString("- " + memory + "\n")
		}
	}
	if len(records) > 0 {
		if text.Len() > 0 {
			text.WriteByte('\n')
		}
		text.WriteString("## 過去の会話\n" + historyLegend + "\n")
		for _, record := range records {
			text.WriteString(contextRecord(record))
			text.WriteByte('\n')
		}
	}
	return text.String()
}

// contextRecord は記録を1行にする。ID や世代などの内部の項目と、音声の未送信部分などのメタデータは渡さない。
// 本文の改行はそのまま残す。日時で始まる行が記録の区切りになる。
func contextRecord(record types.ConversationRecord) string {
	label := record.Role
	// ツールの本文はツール名で始まるので、出どころを重ねない。
	if record.Source != "" && record.Source != "realtime" && record.Role != types.RoleTool {
		label += "(" + record.Source + ")"
	}
	return "[" + record.CreatedAt.In(japanLocation).Format("01/02 15:04") + "] " + label + ": " + record.Text
}
