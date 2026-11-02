package userlocation

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestStoreUpdatesLatestLocation(t *testing.T) {
	var store Store
	if got := store.Snapshot(); got != nil {
		t.Fatalf("初期値 = %#v、期待値はnil", got)
	}
	for _, place := range []string{"home", "会社", "会社"} {
		before := time.Now().UTC()
		record := store.Update(place)
		after := time.Now().UTC()
		if record.Place != place || record.UpdatedAt.Before(before) || record.UpdatedAt.After(after) {
			t.Fatalf("更新結果が場所と受信時刻に一致しない: %#v", record)
		}
		if got := store.Snapshot(); got == nil || *got != record {
			t.Fatalf("取得値 = %#v、期待値 = %#v", got, record)
		}
	}
	copy := store.Snapshot()
	copy.Place = "変更"
	if got := store.Snapshot(); got.Place != "会社" {
		t.Fatalf("取得値の変更で保存済みデータが変わった: %#v", got)
	}
	var restarted Store
	if got := restarted.Snapshot(); got != nil {
		t.Fatalf("別Storeに現在地が引き継がれた: %#v", got)
	}
}

func TestStoreConcurrentAccess(t *testing.T) {
	var store Store
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				store.Update(fmt.Sprintf("place-%d", i))
				if got := store.Snapshot(); got == nil || got.Place == "" || got.UpdatedAt.IsZero() {
					t.Errorf("不完全な現在地を取得した: %#v", got)
				}
			}
		}()
	}
	wg.Wait()
}
