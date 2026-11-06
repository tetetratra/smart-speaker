package conversation

import (
	"fmt"
	"strings"
	"time"
)

var japanLocation = time.FixedZone("Asia/Tokyo", 9*60*60)

// WithCurrentTime はサーバーのタイムゾーンに依存せず、相対日付の基準を付ける。
func WithCurrentTime(instructions string, now time.Time) string {
	now = now.In(japanLocation)
	return strings.TrimSpace(instructions) + fmt.Sprintf(`

## 現在の日時
現在日時: %s
タイムゾーン: Asia/Tokyo（日本時間、UTC+09:00）
今日: %s
明日: %s
昨日: %s
「今日」「明日」「現在」はこの日時を基準に解釈する。過去の会話や検索結果内の日付を現在日時として扱わない。
`, now.Format(time.RFC3339), now.Format(time.DateOnly), now.AddDate(0, 0, 1).Format(time.DateOnly), now.AddDate(0, 0, -1).Format(time.DateOnly))
}
