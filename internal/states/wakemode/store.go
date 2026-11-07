package wakemode

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// ConversationTimeoutは、呼びかけ後に呼びかけなしで話せる時間。最後のユーザー発話から数える。
const ConversationTimeout = time.Minute

// DefaultPatternsは既定の呼びかけ。文頭から4文字以内の前置き（「ねえ」など）を許す。
var DefaultPatterns = []string{`^.{0,4}(起きて|おきて|オキテ|掟)`}

const (
	maxPatterns      = 16
	maxPatternLength = 200
)

type Config struct {
	Required      bool     `json:"required"`
	Patterns      []string `json:"patterns"`
	MinConfidence float64  `json:"min_confidence"`
}
type Snapshot struct {
	Config
	Last     *Decision `json:"last,omitempty"`
	Revision uint64    `json:"revision"`
	// Awakeは呼びかけ後の会話中で、呼びかけなしの発話も受け付ける状態。保存しない。
	Awake      bool       `json:"awake"`
	AwakeUntil *time.Time `json:"awake_until,omitempty"`
}

// Gatedは、発話ごとに呼びかけの判定が必要な状態か。
func (s Snapshot) Gated() bool { return s.Required && !s.Awake }

type Store struct {
	mu         sync.Mutex
	path       string
	config     Config
	patterns   []*regexp.Regexp
	last       *Decision
	revision   uint64
	awakeUntil time.Time
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path, config: Config{Patterns: append([]string(nil), DefaultPatterns...), MinConfidence: 0.55}, revision: 1}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if err = json.Unmarshal(data, &s.config); err != nil {
			return nil, err
		}
		// 旧形式（word・min_similarity）の保存内容は呼びかけを持たないため、既定の呼びかけを使う。
		if len(s.config.Patterns) == 0 {
			s.config.Patterns = append([]string(nil), DefaultPatterns...)
		}
	}
	if s.patterns, err = validConfig(s.config); err != nil {
		return nil, err
	}
	return s, nil
}

// normalizeは認識テキストの空白・句読点を除き、カタカナをひらがな、英字を小文字へそろえる。
func normalize(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return -1
		}
		return unicode.ToLower(toHiragana(r))
	}, text)
}
func toHiragana(r rune) rune {
	if r >= 'ァ' && r <= 'ヶ' {
		return r - 0x60
	}
	return r
}

// {,n} はGoの正規表現では繰り返しにならず文字列として扱われ、呼びかけが一致しなくなる。
var unsupportedRepeat = regexp.MustCompile(`(^|[^\\])\{,\d*\}`)

// compilePatternは、認識テキストと同じくカタカナをひらがなへそろえ、英字の大小を区別せずに解釈する。
func compilePattern(pattern string) (*regexp.Regexp, error) {
	if strings.TrimSpace(pattern) == "" || utf8.RuneCountInString(pattern) > maxPatternLength {
		return nil, fmt.Errorf("呼びかけのパターンを1〜%d文字で指定してください", maxPatternLength)
	}
	if unsupportedRepeat.MatchString(pattern) {
		return nil, fmt.Errorf("呼びかけのパターン %q の {,n} は繰り返しとして扱われません。{0,n} と書いてください", pattern)
	}
	re, err := regexp.Compile("(?i)" + strings.Map(toHiragana, pattern))
	if err != nil {
		return nil, fmt.Errorf("呼びかけのパターン %q を解釈できません: %w", pattern, err)
	}
	return re, nil
}
func validConfig(c Config) ([]*regexp.Regexp, error) {
	if len(c.Patterns) == 0 || len(c.Patterns) > maxPatterns {
		return nil, fmt.Errorf("呼びかけのパターンを1〜%d件で指定してください", maxPatterns)
	}
	patterns := make([]*regexp.Regexp, 0, len(c.Patterns))
	for _, pattern := range c.Patterns {
		re, err := compilePattern(pattern)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, re)
	}
	if math.IsNaN(c.MinConfidence) || math.IsInf(c.MinConfidence, 0) || c.MinConfidence <= 0 || c.MinConfidence > 1 {
		return nil, fmt.Errorf("信頼度のしきい値は0より大きく1以下で指定してください")
	}
	return patterns, nil
}
func (s *Store) snapshotLocked(now time.Time) Snapshot {
	snapshot := Snapshot{Config: s.config, Last: s.last, Revision: s.revision}
	snapshot.Patterns = append([]string(nil), s.config.Patterns...)
	if s.config.Required && now.Before(s.awakeUntil) {
		until := s.awakeUntil
		snapshot.Awake = true
		snapshot.AwakeUntil = &until
	}
	return snapshot
}
func (s *Store) Snapshot(now time.Time) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked(now)
}
func (s *Store) Set(required bool, patterns []string, now time.Time) (Snapshot, error) {
	return s.SetOptions(required, patterns, nil, now)
}

// SetOptionsは設定を保存する。patternsが空なら呼びかけを変更しない。設定を変えると会話中の状態は終わる。
func (s *Store) SetOptions(required bool, patterns []string, confidence *float64, now time.Time) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.config
	next.Required = required
	if len(patterns) > 0 {
		next.Patterns = append([]string(nil), patterns...)
	}
	if confidence != nil {
		next.MinConfidence = *confidence
	}
	compiled, err := validConfig(next)
	if err != nil {
		return Snapshot{}, err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return Snapshot{}, err
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return Snapshot{}, err
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), ".wake-mode-*")
	if err != nil {
		return Snapshot{}, err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return Snapshot{}, err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return Snapshot{}, err
	}
	if err = file.Close(); err != nil {
		return Snapshot{}, err
	}
	if err = os.Rename(file.Name(), s.path); err != nil {
		return Snapshot{}, err
	}
	s.config = next
	s.patterns = compiled
	s.last = nil
	s.awakeUntil = time.Time{}
	s.revision++
	return s.snapshotLocked(now), nil
}

type Decision struct {
	Confidence float64 `json:"confidence"`
	// Matchedは呼びかけのパターンに一致したか。会話中は一致しなくても受け付ける。
	Matched  bool `json:"matched"`
	Accepted bool `json:"accepted"`
	Failed   bool `json:"failed"`
	// WakeOnlyは発話が呼びかけだけで、依頼の本文を含まないこと。
	WakeOnly bool `json:"wake_only"`
}

// Evaluateは発話ごとに判定する。受け付けた発話は会話中の時間を延ばす。拒否された発話の内容は保持しない。
func (s *Store) Evaluate(text string, confidence float64, revision uint64, failed bool, now time.Time) Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := Decision{Confidence: confidence, Failed: failed}
	if !failed {
		d.Matched, d.WakeOnly = match(s.patterns, text)
	}
	awake := now.Before(s.awakeUntil)
	d.Accepted = s.config.Required && s.revision == revision && !failed && confidence >= s.config.MinConfidence && confidence <= 1 && (d.Matched || awake)
	if s.revision == revision {
		s.last = &d
	}
	if d.Accepted {
		s.awakeUntil = now.Add(ConversationTimeout)
	}
	return d
}

// Touchは会話中のユーザー発話で、会話中の時間を最後の発話から数え直す。会話中でなければ何もしない。
func (s *Store) Touch(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.config.Required && now.Before(s.awakeUntil) {
		s.awakeUntil = now.Add(ConversationTimeout)
	}
}

// matchは正規化したテキストを各パターンで調べ、一致した部分より後に本文が残らなければ呼びかけだけとみなす。
func match(patterns []*regexp.Regexp, text string) (matched, wakeOnly bool) {
	normalized := normalize(text)
	if normalized == "" {
		return false, false
	}
	for _, re := range patterns {
		loc := re.FindStringIndex(normalized)
		if loc == nil {
			continue
		}
		if loc[1] == len(normalized) {
			return true, true
		}
		matched = true
	}
	return matched, false
}
