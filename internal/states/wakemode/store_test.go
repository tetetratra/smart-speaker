package wakemode

import (
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestEveryUtteranceRequiresWakeWordAndConfidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mode.json")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot(time.Now()).Required {
		t.Fatal("既定が必須です")
	}
	m, err := s.Set(true, "スピーカー", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		text       string
		confidence float64
		accepted   bool
	}{
		{"すぴーかー、今日の予定は", 0.9, true}, {"スピカ―、今日の予定は", 0.9, false},
		{"スビーカー、教えて", 0.9, true}, {"続きはどうですか", 0.9, false}, {"スピーカー、教えて", 0.1, false},
	} {
		d := s.Evaluate(test.text, test.confidence, m.Revision, false)
		if d.Accepted != test.accepted {
			t.Fatal(test, d)
		}
	}
	restored, err := NewStore(path)
	if err != nil || !restored.Snapshot(time.Now()).Required {
		t.Fatal(err)
	}
	if d := s.Evaluate("スピーカー", 0.9, m.Revision-1, false); d.Accepted {
		t.Fatal("旧設定で受理しました")
	}
	if _, err := s.SetOptions(true, "", nil, ptr(math.NaN()), time.Now()); err == nil {
		t.Fatal("不正なしきい値を受理しました")
	}
}
func ptr(v float64) *float64 { return &v }
