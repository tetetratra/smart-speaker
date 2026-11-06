package conversation

import (
	"strings"
	"testing"
	"time"
)

func TestWithCurrentTimeUsesJapanDateAcrossMidnight(t *testing.T) {
	for _, tc := range []struct {
		name, utc, today, tomorrow, yesterday string
	}{
		{"本番で誤った深夜の日付", "2026-11-05T16:46:00Z", "2026-11-06", "2026-11-07", "2026-11-05"},
		{"年末の深夜前", "2026-12-31T14:59:59Z", "2026-12-31", "2027-01-01", "2026-12-30"},
		{"年明けの深夜", "2026-12-31T15:00:00Z", "2027-01-01", "2027-01-02", "2026-12-31"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tc.utc)
			if err != nil {
				t.Fatal(err)
			}
			got := WithCurrentTime("元の指示と復元した履歴", now)
			for _, want := range []string{"元の指示と復元した履歴", "Asia/Tokyo", "+09:00", "今日: " + tc.today, "明日: " + tc.tomorrow, "昨日: " + tc.yesterday} {
				if !strings.Contains(got, want) {
					t.Fatalf("日時の指示に %q がありません: %s", want, got)
				}
			}
		})
	}
}
