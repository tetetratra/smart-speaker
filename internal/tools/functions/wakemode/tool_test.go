package wakemode

import (
	"encoding/json"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"path/filepath"
	"testing"
	"time"
)

// 話しかけて必須モードは切り替えられるが、呼びかけの言葉やしきい値は変えられない。
func TestSetWakeModeOnlySwitchesRequired(t *testing.T) {
	store, err := wakemode.NewStore(filepath.Join(t.TempDir(), "mode.json"), wakemode.DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	tool := &Tool{Store: store}
	run := func(raw string) error {
		var args map[string]any
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			t.Fatal(err)
		}
		_, err := tool.Run(args)
		return err
	}
	if err := run(`{"required":true}`); err != nil || !store.Snapshot(time.Now()).Required {
		t.Fatal(err)
	}
	for _, raw := range []string{`{"required":true,"words":["ねえ"]}`, `{"required":true,"min_similarity":0.6}`, `{"required":"true"}`} {
		if err := run(raw); err == nil {
			t.Fatal("不正な引数を受理しました", raw)
		}
	}
	if mode := store.Snapshot(time.Now()); len(mode.Words) != len(wakemode.DefaultWords) || mode.MinSimilarity != 0.65 {
		t.Fatal("呼びかけの設定が変わりました", mode)
	}
}
