package wakemode

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultWordsAcceptWakeWordNearSentenceStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mode.json")
	s, err := NewStore(path, DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot(time.Now()).Required {
		t.Fatal("既定が必須です")
	}
	m, err := s.SetRequired(true, time.Now())
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
		// 書き起こしの揺れ（1文字違い）は許す。
		{"置きて", 0.9, true, true},
		{"起きで、天気は", 0.9, true, false},
		{"えっとね、あのさ起きて", 0.9, false, false},
		{"明日は早く家を出るので起きて", 0.9, false, false},
		{"会議の続きはどうですか", 0.9, false, false},
		{"起きて、教えて", 0.1, false, false},
	} {
		now = now.Add(2 * ConversationTimeout)
		d := s.Evaluate(test.text, test.confidence, m.Revision, false, now)
		if d.Accepted != test.accepted || d.WakeOnly != test.wakeOnly {
			t.Fatal(test, d)
		}
	}
	restored, err := NewStore(path, DefaultSettings())
	if err != nil || !restored.Snapshot(time.Now()).Required {
		t.Fatal(err)
	}
	if d := s.Evaluate("起きて", 0.9, m.Revision-1, false, now.Add(2*ConversationTimeout)); d.Accepted {
		t.Fatal("旧設定で受理しました")
	}
}

func TestWakeKeepsConversationUntilOneMinuteAfterLastUtterance(t *testing.T) {
	s, _ := NewStore(filepath.Join(t.TempDir(), "mode.json"), DefaultSettings())
	m, _ := s.SetRequired(true, time.Now())
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
	if _, err := s.SetRequired(true, now.Add(301*time.Second)); err != nil {
		t.Fatal(err)
	}
	if !s.Snapshot(now.Add(302 * time.Second)).Gated() {
		t.Fatal("設定変更後も会話中のままです")
	}
}

func TestSettingsAreValidatedAndOnlyRequiredIsPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mode.json")
	for _, settings := range []Settings{
		{Words: []string{"！？"}, MaxPrefix: 4, MinConfidence: 0.5, MinSimilarity: 0.5},
		{Words: []string{"ねえ"}, MaxPrefix: -1, MinConfidence: 0.5, MinSimilarity: 0.5},
		{Words: []string{"ねえ"}, MaxPrefix: 21, MinConfidence: 0.5, MinSimilarity: 0.5},
		{Words: []string{"ねえ"}, MaxPrefix: 4, MinConfidence: math.NaN(), MinSimilarity: 0.5},
		{Words: []string{"ねえ"}, MaxPrefix: 4, MinConfidence: 0.5, MinSimilarity: 0},
		{MaxPrefix: 4, MinConfidence: 0.5, MinSimilarity: 0.5},
	} {
		if _, err := NewStore(path, settings); err == nil {
			t.Fatal("不正な設定を受理しました", settings)
		}
	}
	settings := Settings{Words: []string{"ヘイ", "hello"}, MaxPrefix: 0, MinConfidence: 0.55, MinSimilarity: 1}
	s, err := NewStore(path, settings)
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.SetRequired(true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if d := s.Evaluate("へい、天気", 0.9, m.Revision, false, now); !d.Accepted {
		t.Fatal("カタカナの言葉がひらがなの認識結果に一致しません", d)
	}
	if d := s.Evaluate("Hello, weather", 0.9, m.Revision, false, now.Add(2*ConversationTimeout)); !d.Accepted {
		t.Fatal("英字の大小を区別しました", d)
	}
	if d := s.Evaluate("ねえヘイ", 0.9, m.Revision, false, now.Add(4*ConversationTimeout)); d.Accepted {
		t.Fatal("許容文字数を超える前置きを許しました", d)
	}
	// 保存するのは必須モードだけで、判定の設定は起動時に渡した値を使う。
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "words") {
		t.Fatal(err, string(data))
	}
	restored, err := NewStore(path, DefaultSettings())
	mode := restored.Snapshot(time.Now())
	if err != nil || !mode.Required || len(mode.Words) != len(DefaultWords) || mode.MaxPrefix != 4 {
		t.Fatal(err, mode)
	}
}

// 以前の保存内容に言葉やしきい値があっても無視し、必須モードだけを引き継ぐ。
func TestLegacySavedSettingsAreIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mode.json")
	if err := os.WriteFile(path, []byte(`{"required":true,"words":["ねえ"],"max_prefix":0,"min_confidence":0.9,"min_similarity":0.9}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(path, DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	mode := s.Snapshot(time.Now())
	if !mode.Required || mode.MinConfidence != 0.55 || len(mode.Words) != len(DefaultWords) || mode.MaxPrefix != 4 || mode.MinSimilarity != 0.65 {
		t.Fatal(mode)
	}
}

func TestAwakenStartsConversationOnlyWhenRequired(t *testing.T) {
	s, _ := NewStore(filepath.Join(t.TempDir(), "mode.json"), DefaultSettings())
	now := time.Now()
	s.Awaken(now)
	if s.Snapshot(now).Awake {
		t.Fatal("呼びかけ不要のときに会話中になりました")
	}
	if _, err := s.SetRequired(true, now); err != nil {
		t.Fatal(err)
	}
	// アシスタントから質問したときは、呼びかけなしで返事を受け付ける。
	s.Awaken(now)
	if s.Snapshot(now.Add(59 * time.Second)).Gated() {
		t.Fatal("質問の後に会話中になりません")
	}
	if !s.Snapshot(now.Add(61 * time.Second)).Gated() {
		t.Fatal("1分たっても会話中のままです")
	}
}
