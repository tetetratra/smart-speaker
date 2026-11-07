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
func TestPersistedOneShotRunsOnlyOnceAndRetainsNaturalInstruction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	var deliveries []Delivery
	deliver := func(_ context.Context, d Delivery) error { deliveries = append(deliveries, d); return nil }
	service, err := New(path, deliver)
	if err != nil {
		t.Fatal(err)
	}
	now := stamp("2026-11-06T08:00:00+09:00")
	job, err := service.Create(Spec{Name: "目覚まし", Instruction: "ユーザーを起こして", Schedule: Schedule{AfterSeconds: 600}, Speak: true}, now)
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
	if len(deliveries) != 1 || !deliveries[0].Speak || deliveries[0].Instruction != "ユーザーを起こして" {
		t.Fatal(deliveries)
	}
	if len(again.List()) != 0 {
		t.Fatal("実行済みの日時指定の予約が残りました", again.List())
	}
}
func TestFailedOneShotIsAlsoDeleted(t *testing.T) {
	service, _ := New(filepath.Join(t.TempDir(), "jobs.json"), func(context.Context, Delivery) error { return errors.New("受付できません") })
	now := stamp("2026-11-06T08:00:00+09:00")
	if _, err := service.Create(Spec{Name: "室温", Instruction: "室温を伝える", Schedule: Schedule{AfterSeconds: 60}, Speak: true}, now); err != nil {
		t.Fatal(err)
	}
	if err := service.RunDue(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(service.List()) != 0 {
		t.Fatal(service.List())
	}
}
func TestLegacyFinishedOneShotIsDroppedOnLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	legacy := `[
  {"id":"done","name":"済み","instruction":"x","schedule":{"at":"2026-11-07T11:07:08+09:00","timezone":"Asia/Tokyo"},"speak":true,"enabled":false,"last_status":"submitted"},
  {"id":"cron","name":"毎朝","instruction":"y","schedule":{"cron":"0 8 * * *","timezone":"Asia/Tokyo"},"speak":false,"enabled":true,"next_run_at":"2026-11-08T08:00:00+09:00","last_status":"scheduled"}
]`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	service, err := New(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if jobs := service.List(); len(jobs) != 1 || jobs[0].ID != "cron" {
		t.Fatal(jobs)
	}
	restored, err := New(path, nil)
	if err != nil || len(restored.List()) != 1 {
		t.Fatal(err, restored.List())
	}
}
func TestMissedCronCoalescesAndConcurrentTicksDoNotDuplicate(t *testing.T) {
	var count atomic.Int32
	service, _ := New(filepath.Join(t.TempDir(), "jobs.json"), func(_ context.Context, d Delivery) error {
		if d.Speak {
			t.Error("無音設定が失われました")
		}
		count.Add(1)
		return nil
	})
	now := stamp("2026-11-02T07:00:00+09:00")
	_, err := service.Create(Spec{Name: "仕事モード", Instruction: "ウェイクアップワードを必要にして", Schedule: Schedule{Cron: "0 8 * * 1-5"}}, now)
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
	spec := Spec{Name: "予約", Instruction: "実行して", Schedule: Schedule{AfterSeconds: 1}}
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
	spec.Instruction = "変更後"
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
