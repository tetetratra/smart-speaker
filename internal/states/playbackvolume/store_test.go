package playbackvolume

import "testing"

func TestStoreSetAndStep(t *testing.T) {
	var store Store
	if got := store.Level(); got != Default {
		t.Fatalf("初期値 = %s、期待値は%s", got, Default)
	}
	changed, unsubscribe := store.Subscribe()
	defer unsubscribe()

	if previous, ok := store.Set("low"); !ok || previous != Default || store.Level() != "low" {
		t.Fatalf("Setが反映されない: previous=%s ok=%v level=%s", previous, ok, store.Level())
	}
	select {
	case <-changed:
	default:
		t.Fatal("変更が通知されない")
	}
	if _, ok := store.Set("loud"); ok || store.Level() != "low" {
		t.Fatal("段階にない値を受け付けた")
	}

	if previous, current := store.Step(-1); previous != "low" || current != "quiet" {
		t.Fatalf("1段階下げられない: %s -> %s", previous, current)
	}
	// 端では止まり、変わらなければ通知しない。
	store.Step(-10)
	<-changed
	if previous, current := store.Step(-1); previous != "mute" || current != "mute" {
		t.Fatalf("下端で止まらない: %s -> %s", previous, current)
	}
	select {
	case <-changed:
		t.Fatal("変わっていないのに通知した")
	default:
	}
	if _, current := store.Step(10); current != "boost" {
		t.Fatalf("上端で止まらない: %s", current)
	}
}
