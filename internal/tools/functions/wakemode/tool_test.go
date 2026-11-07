package wakemode

import (
	"encoding/json"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"path/filepath"
	"testing"
	"time"
)

// 話しかけて判定の設定を調整できるよう、LLMが送るJSONの引数をそのまま受け付ける。
func TestSetWakeModeUpdatesThresholds(t *testing.T) {
	store, err := wakemode.NewStore(filepath.Join(t.TempDir(), "mode.json"))
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
	if err := run(`{"required":true,"words":["ねえ"],"max_prefix":2,"min_similarity":0.6,"min_confidence":0.4}`); err != nil {
		t.Fatal(err)
	}
	mode := store.Snapshot(time.Now())
	if !mode.Required || mode.Words[0] != "ねえ" || mode.MaxPrefix != 2 || mode.MinSimilarity != 0.6 || mode.MinConfidence != 0.4 {
		t.Fatal(mode)
	}
	for _, raw := range []string{`{"required":true,"max_prefix":1.5}`, `{"required":true,"min_similarity":"0.6"}`, `{"required":true,"patterns":["^起きて"]}`} {
		if err := run(raw); err == nil {
			t.Fatal("不正な引数を受理しました", raw)
		}
	}
}
