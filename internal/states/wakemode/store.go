package wakemode

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// ConversationTimeoutは、呼びかけ後に呼びかけなしで話せる時間。最後のユーザー発話から数える。
const ConversationTimeout = time.Minute

// DefaultWordsは既定の呼びかけ。STTが漢字・かなのどちらで書き起こしても通るよう両方を持つ。
var DefaultWords = []string{"おきて", "起きて"}

const (
	maxWords      = 16
	maxWordLength = 64
	maxPrefixMax  = 20
)

type Config struct {
	Required bool     `json:"required"`
	Words    []string `json:"words"`
	// MaxPrefixは、呼びかけの前に許す文字数（「ねえ」などの前置き）。正規化後の文字数で数える。
	MaxPrefix     int     `json:"max_prefix"`
	MinConfidence float64 `json:"min_confidence"`
	MinSimilarity float64 `json:"min_similarity"`
}

// Optionsは設定の部分更新。nilの項目は変更しない。
type Options struct {
	Words         []string
	MaxPrefix     *int
	MinConfidence *float64
	MinSimilarity *float64
}

func defaultConfig() Config {
	return Config{Words: append([]string(nil), DefaultWords...), MaxPrefix: 4, MinConfidence: 0.55, MinSimilarity: 0.65}
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
	last       *Decision
	revision   uint64
	awakeUntil time.Time
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path, config: defaultConfig(), revision: 1}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		if err = json.Unmarshal(data, &s.config); err != nil {
			return nil, err
		}
		// 旧形式（wordやpatterns）の保存内容は言葉を持たないため、既定の呼びかけと許容文字数を使う。
		if len(s.config.Words) == 0 {
			defaults := defaultConfig()
			s.config.Words = defaults.Words
			s.config.MaxPrefix = defaults.MaxPrefix
		}
		if s.config.MinSimilarity == 0 {
			s.config.MinSimilarity = defaultConfig().MinSimilarity
		}
	}
	if err = validConfig(s.config); err != nil {
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

func validConfig(c Config) error {
	if len(c.Words) == 0 || len(c.Words) > maxWords {
		return fmt.Errorf("呼びかけの言葉を1〜%d件で指定してください", maxWords)
	}
	for _, word := range c.Words {
		if normalize(word) == "" || utf8.RuneCountInString(word) > maxWordLength {
			return fmt.Errorf("呼びかけの言葉を1〜%d文字で指定してください", maxWordLength)
		}
	}
	if c.MaxPrefix < 0 || c.MaxPrefix > maxPrefixMax {
		return fmt.Errorf("呼びかけの前に許す文字数は0〜%dで指定してください", maxPrefixMax)
	}
	for _, v := range []float64{c.MinConfidence, c.MinSimilarity} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 1 {
			return fmt.Errorf("判定のしきい値は0より大きく1以下で指定してください")
		}
	}
	return nil
}
func (s *Store) snapshotLocked(now time.Time) Snapshot {
	snapshot := Snapshot{Config: s.config, Last: s.last, Revision: s.revision}
	snapshot.Words = append([]string(nil), s.config.Words...)
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
func (s *Store) Set(required bool, words []string, now time.Time) (Snapshot, error) {
	return s.SetOptions(required, Options{Words: words}, now)
}

// SetOptionsは設定を保存する。指定のない項目は変更しない。設定を変えると会話中の状態は終わる。
func (s *Store) SetOptions(required bool, options Options, now time.Time) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.config
	next.Required = required
	if len(options.Words) > 0 {
		next.Words = make([]string, 0, len(options.Words))
		for _, word := range options.Words {
			next.Words = append(next.Words, strings.TrimSpace(word))
		}
	}
	if options.MaxPrefix != nil {
		next.MaxPrefix = *options.MaxPrefix
	}
	if options.MinConfidence != nil {
		next.MinConfidence = *options.MinConfidence
	}
	if options.MinSimilarity != nil {
		next.MinSimilarity = *options.MinSimilarity
	}
	if err := validConfig(next); err != nil {
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
	s.last = nil
	s.awakeUntil = time.Time{}
	s.revision++
	return s.snapshotLocked(now), nil
}

type Decision struct {
	Confidence float64 `json:"confidence"`
	// Similarityは文頭付近で最も呼びかけに近い部分の類似度（0〜1）。
	Similarity float64 `json:"similarity"`
	// Matchedは類似度がしきい値以上だったか。会話中は一致しなくても受け付ける。
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
		d.Similarity, d.WakeOnly = match(text, s.config.Words, s.config.MaxPrefix, s.config.MinSimilarity)
		d.Matched = d.Similarity >= s.config.MinSimilarity
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

// matchは、正規化したテキストの先頭maxPrefix文字以内から始まる部分と各言葉の類似度を調べ、最大値を返す。
// しきい値以上の部分がテキストの末尾まで届いていれば、本文のない呼びかけだけの発話とみなす。
func match(text string, words []string, maxPrefix int, minSimilarity float64) (best float64, wakeOnly bool) {
	a := []rune(normalize(text))
	for _, word := range words {
		b := []rune(normalize(word))
		if len(a) == 0 || len(b) == 0 {
			continue
		}
		for start := 0; start <= maxPrefix && start < len(a); start++ {
			// 書き起こしの脱字・余分な文字を許すため、言葉の長さ±2文字の部分を比べる。
			for length := max(1, len(b)-2); length <= len(b)+2 && start+length <= len(a); length++ {
				score := similarity(a[start:start+length], b)
				best = max(best, score)
				if score >= minSimilarity && start+length == len(a) {
					wakeOnly = true
				}
			}
		}
	}
	return best, wakeOnly
}

// similarityは編集距離を長いほうの文字数で割り、0〜1の類似度へ変換する。
func similarity(x, b []rune) float64 {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i, r := range x {
		next := make([]int, len(b)+1)
		next[0] = i + 1
		for j, q := range b {
			cost := 0
			if r != q {
				cost = 1
			}
			next[j+1] = min(next[j]+1, prev[j+1]+1, prev[j]+cost)
		}
		prev = next
	}
	return 1 - float64(prev[len(b)])/float64(max(len(x), len(b)))
}
