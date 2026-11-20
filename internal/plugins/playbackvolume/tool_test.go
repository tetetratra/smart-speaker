package playbackvolume

import (
	"context"
	"testing"
)

func TestSetPlaybackVolume(t *testing.T) {
	store := &Store{}
	tool := &Tool{Store: store}
	run := func(args map[string]any) (map[string]any, error) {
		return tool.Run(context.Background(), args)
	}
	if got, err := run(map[string]any{"level": "quiet"}); err != nil || got["previous"] != "low" || got["level"] != "quiet" {
		t.Fatalf("段階を指定して変えられない: %v %v", got, err)
	}
	if got, err := run(map[string]any{"level": "up"}); err != nil || got["previous"] != "quiet" || got["level"] != "low" {
		t.Fatalf("1段階上げられない: %v %v", got, err)
	}
	if got, err := run(map[string]any{"level": "down"}); err != nil || got["level"] != "quiet" || store.Level() != "quiet" {
		t.Fatalf("1段階下げられない: %v %v", got, err)
	}
	for _, args := range []map[string]any{{}, {"level": "loud"}, {"level": 1}, {"level": "low", "device": "a"}} {
		if _, err := run(args); err == nil {
			t.Fatalf("不正な引数を受け付けた: %v", args)
		}
	}
	if store.Level() != "quiet" {
		t.Fatal("不正な引数で音量が変わった")
	}
}
