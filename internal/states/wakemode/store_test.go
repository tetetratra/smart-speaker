package wakemode

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultPatternAcceptsWakeWordAtSentenceStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mode.json")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot(time.Now()).Required {
		t.Fatal("既定が必須です")
	}
	m, err := s.Set(true, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// 判定ごとに会話中の状態を持ち越さないよう、受理のたびに1分以上進める。
	now := time.Now()
	for _, test := range []struct {
		text               string
		confidence         float64
		accepted, wakeOnly bool
	}{
		{"起きて、今日の予定は", 0.9, true, false},
		{"ねえ、おきて。天気は？", 0.9, true, false},
		{"オキテ", 0.9, true, true},
		{"掟。", 0.9, true, true},
		{"えっとね、あのさ起きて", 0.9, false, false},
		{"会議の続きはどうですか", 0.9, false, false},
		{"起きて、教えて", 0.1, false, false},
	} {
		now = now.Add(2 * ConversationTimeout)
		d := s.Evaluate(test.text, test.confidence, m.Revision, false, now)
		if d.Accepted != test.accepted || d.WakeOnly != test.wakeOnly {
			t.Fatal(test, d)
		}
	}
	restored, err := NewStore(path)
	if err != nil || !restored.Snapshot(time.Now()).Required {
		t.Fatal(err)
	}
	if d := s.Evaluate("起きて", 0.9, m.Revision-1, false, now.Add(2*ConversationTimeout)); d.Accepted {
		t.Fatal("旧設定で受理しました")
	}
	if _, err := s.SetOptions(true, nil, ptr(math.NaN()), time.Now()); err == nil {
		t.Fatal("不正なしきい値を受理しました")
	}
}

func TestWakeKeepsConversationUntilOneMinuteAfterLastUtterance(t *testing.T) {
	s, _ := NewStore(filepath.Join(t.TempDir(), "mode.json"))
	m, _ := s.Set(true, nil, time.Now())
	now := time.Now()
	if !s.Snapshot(now).Gated() {
		t.Fatal("呼びかけ前から会話中です")
	}
	if d := s.Evaluate("起きて", 0.9, m.Revision, false, now); !d.Accepted {
		t.Fatal(d)
	}
	if mode := s.Snapshot(now.Add(59 * time.Second)); mode.Gated() || !mode.Awake {
		t.Fatal("呼びかけ直後に会話中になりません", mode)
	}
	// 会話中は呼びかけのない発話も受け付け、最後の発話から1分を数え直す。
	if d := s.Evaluate("続きも教えて", 0.9, m.Revision, false, now.Add(50*time.Second)); !d.Accepted {
		t.Fatal(d)
	}
	s.Touch(now.Add(100 * time.Second))
	if s.Snapshot(now.Add(159 * time.Second)).Gated() {
		t.Fatal("最後の発話から1分以内に会話が終わりました")
	}
	if !s.Snapshot(now.Add(160 * time.Second)).Gated() {
		t.Fatal("無音が1分続いても会話中のままです")
	}
	// 会話が終わった後のTouchでは再開しない。
	s.Touch(now.Add(200 * time.Second))
	if !s.Snapshot(now.Add(201 * time.Second)).Gated() {
		t.Fatal("呼びかけなしで会話が再開しました")
	}
	// 設定を変えると会話中の状態は終わり、保存もしない。
	s.Evaluate("起きて", 0.9, m.Revision, false, now.Add(300*time.Second))
	if _, err := s.Set(true, []string{`^ねえ`}, now.Add(301*time.Second)); err != nil {
		t.Fatal(err)
	}
	if !s.Snapshot(now.Add(302 * time.Second)).Gated() {
		t.Fatal("設定変更後も会話中のままです")
	}
}

func TestPatternsAreValidatedAndPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mode.json")
	s, _ := NewStore(path)
	for _, patterns := range [][]string{{`^.{,4}起きて`}, {`(`}, {" "}} {
		if _, err := s.Set(true, patterns, time.Now()); err == nil {
			t.Fatal("不正なパターンを受理しました", patterns)
		}
	}
	m, err := s.Set(true, []string{`^ヘイ`, `^hello`}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if d := s.Evaluate("へい、天気", 0.9, m.Revision, false, now); !d.Accepted {
		t.Fatal("カタカナのパターンがひらがなの認識結果に一致しません", d)
	}
	if d := s.Evaluate("Hello, weather", 0.9, m.Revision, false, now.Add(2*ConversationTimeout)); !d.Accepted {
		t.Fatal("英字の大小を区別しました", d)
	}
	restored, err := NewStore(path)
	if err != nil || len(restored.Snapshot(time.Now()).Patterns) != 2 {
		t.Fatal(err, restored.Snapshot(time.Now()))
	}
}

func TestLegacyWordSettingFallsBackToDefaultPatterns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mode.json")
	if err := os.WriteFile(path, []byte(`{"required":true,"word":"スピーカー","min_confidence":0.6,"min_similarity":0.8}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	mode := s.Snapshot(time.Now())
	if !mode.Required || mode.MinConfidence != 0.6 || len(mode.Patterns) != 1 || mode.Patterns[0] != DefaultPatterns[0] {
		t.Fatal(mode)
	}
}
func ptr(v float64) *float64 { return &v }
