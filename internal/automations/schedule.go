package automations

import (
	"fmt"
	"github.com/robfig/cron/v3"
	"strings"
	"time"
	_ "time/tzdata"
)

type Schedule struct {
	AfterSeconds int64     `json:"after_seconds,omitempty"`
	At           time.Time `json:"at,omitzero"`
	Cron         string    `json:"cron,omitempty"`
	Timezone     string    `json:"timezone,omitempty"`
}

func (s Schedule) next(now time.Time, creating bool) (time.Time, error) {
	if s.Timezone == "" {
		s.Timezone = "Asia/Tokyo"
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return time.Time{}, fmt.Errorf("不正なタイムゾーンです")
	}
	count := 0
	if s.AfterSeconds != 0 {
		count++
	}
	if !s.At.IsZero() {
		count++
	}
	if s.Cron != "" {
		count++
	}
	if count != 1 {
		return time.Time{}, fmt.Errorf("after_seconds・at・cronのいずれか1つを指定してください")
	}
	if s.AfterSeconds != 0 {
		if s.AfterSeconds < 1 || s.AfterSeconds > 31536000 {
			return time.Time{}, fmt.Errorf("指定時間後は1秒〜365日で指定してください")
		}
		return now.Add(time.Duration(s.AfterSeconds) * time.Second), nil
	}
	if !s.At.IsZero() {
		if creating && !s.At.After(now) {
			// AIが現在時刻を取り違えても登録し直せるよう、基準にした現在時刻を返す。
			loc, _ := time.LoadLocation(s.Timezone)
			return time.Time{}, fmt.Errorf("過去の日時です（指定: %s、現在: %s）。現在より後の日時を指定してください", s.At.In(loc).Format(time.RFC3339), now.In(loc).Format(time.RFC3339))
		}
		return s.At, nil
	}
	if len(strings.Fields(s.Cron)) != 5 {
		return time.Time{}, fmt.Errorf("cronは分・時・日・月・曜日の5項目で指定してください")
	}
	parsed, err := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow).Parse("CRON_TZ=" + s.Timezone + " " + s.Cron)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron式が不正です: %w", err)
	}
	next := parsed.Next(now)
	if next.IsZero() {
		return next, fmt.Errorf("次の実行日時が見つかりません")
	}
	return next, nil
}
