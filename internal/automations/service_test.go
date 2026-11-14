package automations

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func stamp(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }
func TestCronAndTimezone(t *testing.T) {
	for _, tt := range []struct{ now, cron, want string }{
		{"2026-11-06T07:59:00+09:00", "0 8 * * 1-5", "2026-11-06T08:00:00+09:00"},
		{"2026-11-06T08:00:00+09:00", "0 8 * * 1-5", "2026-11-09T08:00:00+09:00"},
		{"2026-11-06T23:59:00+09:00", "0 0 * * *", "2026-11-07T00:00:00+09:00"},
	} {
		next, err := (Schedule{Cron: tt.cron, Timezone: "Asia/Tokyo"}).next(stamp(tt.now), true)
		if err != nil || !next.Equal(stamp(tt.want)) {
			t.Fatal(next, err, tt.want)
		}
	}
	for _, schedule := range []Schedule{{Cron: "bad"}, {Cron: "* * * * *", AfterSeconds: 5}, {Timezone: "bad-zone", AfterSeconds: 5}, {At: stamp("2020-01-01T00:00:00Z")}, {AfterSeconds: -1}} {
		if _, err := schedule.next(time.Now(), true); err == nil {
			t.Fatal("不正な予約を受理しました", schedule)
		}
	}
}
func TestPersistedOneShotRunsOnlyOnceAndRetainsScript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	var deliveries []Delivery
	deliver := func(_ context.Context, d Delivery) error { deliveries = append(deliveries, d); return nil }
	service, err := New(path, deliver)
	if err != nil {
		t.Fatal(err)
	}
	now := stamp("2026-11-06T08:00:00+09:00")
	job, err := service.Create(Spec{Name: "目覚まし", Script: `remind("ユーザーを起こして")`, Schedule: Schedule{AfterSeconds: 600}}, now)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := New(path, deliver)
	if err != nil {
		t.Fatal(err)
	}
	if restored.List()[0].ID != job.ID || !restored.List()[0].NextRunAt.Equal(now.Add(10*time.Minute)) {
		t.Fatal(restored.List())
	}
	_ = restored.RunDue(context.Background(), now.Add(9*time.Minute))
	if len(deliveries) != 0 {
		t.Fatal("予定前に実行しました")
	}
	_ = restored.RunDue(context.Background(), now.Add(10*time.Minute))
	again, err := New(path, deliver)
	if err != nil {
		t.Fatal(err)
	}
	_ = again.RunDue(context.Background(), now.Add(time.Hour))
	if len(deliveries) != 1 || deliveries[0].Script != `remind("ユーザーを起こして")` {
		t.Fatal(deliveries)
	}
	if len(again.List()) != 0 {
		t.Fatal("実行済みの日時指定の予約が残りました", again.List())
	}
}
func TestFailedOneShotIsAlsoDeleted(t *testing.T) {
	service, _ := New(filepath.Join(t.TempDir(), "jobs.json"), func(context.Context, Delivery) error { return errors.New("受付できません") })
	now := stamp("2026-11-06T08:00:00+09:00")
	if _, err := service.Create(Spec{Name: "室温", Script: `remind("室温を伝える")`, Schedule: Schedule{AfterSeconds: 60}}, now); err != nil {
		t.Fatal(err)
	}
	if err := service.RunDue(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(service.List()) != 0 {
		t.Fatal(service.List())
	}
}
func TestMissedCronCoalescesAndConcurrentTicksDoNotDuplicate(t *testing.T) {
	var count atomic.Int32
	service, _ := New(filepath.Join(t.TempDir(), "jobs.json"), func(context.Context, Delivery) error {
		count.Add(1)
		return nil
	})
	now := stamp("2026-11-02T07:00:00+09:00")
	_, err := service.Create(Spec{Name: "仕事モード", Script: `toolcall("set_wake_word_required", {required:true})`, Schedule: Schedule{Cron: "0 8 * * 1-5"}}, now)
	if err != nil {
		t.Fatal(err)
	}
	missed := stamp("2026-11-05T10:00:00+09:00")
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := service.RunDue(context.Background(), missed); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if count.Load() != 1 || !service.List()[0].NextRunAt.Equal(stamp("2026-11-06T08:00:00+09:00")) {
		t.Fatal(count.Load(), service.List())
	}
}
func TestFailedPersistenceDoesNotExecuteAndCRUDPersists(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "blocked")
	path := filepath.Join(parent, "jobs.json")
	var count int
	service, _ := New(path, func(context.Context, Delivery) error { count++; return errors.New("受付できません") })
	_ = os.WriteFile(parent, []byte("保存を妨げるファイル"), 0600)
	spec := Spec{Name: "予約", Script: `remind("実行して")`, Schedule: Schedule{AfterSeconds: 1}}
	now := time.Now()
	if _, err := service.Create(spec, now); err == nil || len(service.List()) != 0 {
		t.Fatal("保存失敗の予約が残りました")
	}
	_ = os.Remove(parent)
	spec.Schedule = Schedule{Cron: "* * * * *"}
	job, err := service.Create(spec, now)
	if err != nil {
		t.Fatal(err)
	}
	spec.Script = `remind("変更後")`
	if _, err := service.Update(job.ID, spec, now); err != nil {
		t.Fatal(err)
	}
	if err := service.RunDue(context.Background(), now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if service.List()[0].LastStatus != "failed" || service.List()[0].LastError == "" {
		t.Fatal(service.List())
	}
	if err := service.Delete(job.ID); err != nil {
		t.Fatal(err)
	}
	restored, err := New(path, nil)
	if err != nil || len(restored.List()) != 0 {
		t.Fatal(err, restored.List())
	}
}
func TestPastAtErrorIncludesCurrentTime(t *testing.T) {
	now := stamp("2026-11-07T11:40:00+09:00")
	_, err := (Schedule{At: stamp("2026-11-07T02:37:00Z"), Timezone: "Asia/Tokyo"}).next(now, true)
	want := "過去の日時です（指定: 2026-11-07T11:37:00+09:00、現在: 2026-11-07T11:40:00+09:00）。現在より後の日時を指定してください"
	if err == nil || err.Error() != want {
		t.Fatal(err)
	}
}

func TestScriptValidationRejectsSyntaxAndOldFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	service, err := New(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(Spec{Name: "不正", Script: "function (", Schedule: Schedule{AfterSeconds: 10}}, time.Now()); err == nil {
		t.Fatal("構文エラーを保存しました")
	}
	if err := os.WriteFile(path, []byte(`[{"id":"old","name":"旧予約","instruction":"古い指示","schedule":{"cron":"* * * * *"}}]`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path, nil); err == nil {
		t.Fatal("旧形式を受け付けました")
	}
}

func TestEventJobRunsOnMatchingEventAndRemains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	var deliveries []Delivery
	service, err := New(path, func(_ context.Context, d Delivery) error { deliveries = append(deliveries, d); return nil })
	if err != nil {
		t.Fatal(err)
	}
	now := stamp("2026-11-06T08:00:00+09:00")
	if _, err := service.Create(Spec{Name: "落ち着いたら", Script: `remind("x")`, Schedule: Schedule{Event: "EventConversationIdle"}}, now); err == nil {
		t.Fatal("購読できないイベントを受理しました")
	}
	service.SetEvents(map[string]string{"EventConversationIdle": "無操作が続いた"})
	job, err := service.Create(Spec{Name: "落ち着いたら", Script: `remind("x")`, Schedule: Schedule{Event: "EventConversationIdle"}}, now)
	if err != nil || !job.NextRunAt.IsZero() {
		t.Fatal(job, err)
	}
	if _, err := service.Create(Spec{Name: "二重指定", Script: `remind("x")`, Schedule: Schedule{Event: "EventConversationIdle", Cron: "0 8 * * *"}}, now); err == nil {
		t.Fatal("eventとcronの二重指定を受理しました")
	}
	// 時刻では動かない。
	if err := service.RunDue(context.Background(), now.Add(24*time.Hour)); err != nil || len(deliveries) != 0 {
		t.Fatal(deliveries, err)
	}
	// 別のイベントでは動かない。
	if err := service.HandleEvent(context.Background(), "EventOther", nil, now); err != nil || len(deliveries) != 0 {
		t.Fatal(deliveries, err)
	}
	for i := range 2 {
		if err := service.HandleEvent(context.Background(), "EventConversationIdle", []byte(`{"n":1}`), now.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if len(deliveries) != 2 || deliveries[0].ID != job.ID || string(deliveries[0].Event) != `{"n":1}` {
		t.Fatal(deliveries)
	}
	// 動いたあとも消えず、結果が記録される。再起動後も読み込める。
	reloaded, err := New(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	jobs := reloaded.List()
	if len(jobs) != 1 || jobs[0].LastStatus != "succeeded" || jobs[0].Schedule.Event != "EventConversationIdle" {
		t.Fatal(jobs)
	}
}
