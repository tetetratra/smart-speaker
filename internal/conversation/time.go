package conversation

import (
	"fmt"
	"strings"
	"time"
)

var japanLocation = time.FixedZone("Asia/Tokyo", 9*60*60)

// WithCurrentTime はサーバーのタイムゾーンに依存せず、相対日付の基準を付ける。
func WithCurrentTime(instructions string, now time.Time) string {
	return strings.TrimSpace(instructions) + "\n\n## 現在の日時\n" + dates("現在日時", now) +
		"「今日」「明日」「現在」はこの日時を基準に解釈する。過去の会話や検索結果内の日付を現在日時として扱わない。\n"
}

// TimeNotice は、会話の途中に足す日時の知らせ。古い知らせも会話に残るので、いつ時点の日時かを書く。
func TimeNotice(now time.Time) string {
	return dates("この時点の日時", now) +
		"「今日」「明日」「現在」は、会話の中でいちばん新しい日時の知らせを基準に解釈する。過去の会話や検索結果内の日付を現在日時として扱わない。\n"
}

func dates(label string, now time.Time) string {
	now = now.In(japanLocation)
	return fmt.Sprintf(`%s: %s
タイムゾーン: Asia/Tokyo（日本時間、UTC+09:00）
今日: %s
明日: %s
昨日: %s
`, label, now.Format(time.RFC3339), now.Format(time.DateOnly), now.AddDate(0, 0, 1).Format(time.DateOnly), now.AddDate(0, 0, -1).Format(time.DateOnly))
}
