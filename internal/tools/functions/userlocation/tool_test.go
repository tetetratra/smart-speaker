package userlocation

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/states/userlocation"
)

func TestToolReadsLatestRecord(t *testing.T) {
	store := &userlocation.Store{}
	tool := &Tool{Store: store}
	check := func(want *userlocation.Record) {
		t.Helper()
		result, err := tool.Run(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			Location *userlocation.Record `json:"location"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		if want == nil {
			if string(raw) != `{"location":null}` {
				t.Fatalf("未設定の応答が不正: %s", raw)
			}
		} else if decoded.Location == nil || decoded.Location.Place != want.Place || !decoded.Location.UpdatedAt.Equal(want.UpdatedAt) {
			t.Fatalf("最新の記録と不一致: %s", raw)
		}
	}
	check(nil)
	for _, place := range []string{"自宅", "会社"} {
		record := store.Update(place)
		check(&record)
	}
	if _, err := tool.Run(context.Background(), map[string]any{"place": "別の場所"}); err == nil {
		t.Fatal("余分な引数を拒否しませんでした")
	}
	if store.Snapshot().Place != "会社" {
		t.Fatal("保存先が変更されました")
	}
}
