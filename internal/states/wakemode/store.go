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

type Config struct {
	Required      bool    `json:"required"`
	Word          string  `json:"word"`
	MinConfidence float64 `json:"min_confidence"`
	MinSimilarity float64 `json:"min_similarity"`
}
type Snapshot struct {
	Config
	Last     *Decision `json:"last,omitempty"`
	Revision uint64    `json:"revision"`
}
type Store struct {
	mu       sync.Mutex
	path     string
	config   Config
	last     *Decision
	revision uint64
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path, config: Config{Word: "スピーカー", MinConfidence: 0.55, MinSimilarity: 0.8}, revision: 1}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &s.config); err != nil {
		return nil, err
	}
	if err = validConfig(s.config); err != nil {
		return nil, err
	}
	return s, nil
}
func normalize(word string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			return -1
		}
		if r >= 'ァ' && r <= 'ヶ' {
			r -= 0x60
		}
		return unicode.ToLower(r)
	}, word)
}
func validWord(word string) error {
	if normalize(word) == "" || utf8.RuneCountInString(word) > 64 {
		return fmt.Errorf("呼びかけの言葉を1〜64文字で指定してください")
	}
	return nil
}
func (s *Store) snapshotLocked(now time.Time) Snapshot {
	return Snapshot{Config: s.config, Last: s.last, Revision: s.revision}
}
func (s *Store) Snapshot(now time.Time) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked(now)
}
func (s *Store) Set(required bool, word string, now time.Time) (Snapshot, error) {
	return s.SetOptions(required, word, nil, nil, now)
}
func (s *Store) SetOptions(required bool, word string, confidence, similarity *float64, now time.Time) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.config
	next.Required = required
	if word != "" {
		next.Word = strings.TrimSpace(word)
	}
	if confidence != nil {
		next.MinConfidence = *confidence
	}
	if similarity != nil {
		next.MinSimilarity = *similarity
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
	s.revision++
	return s.snapshotLocked(now), nil
}

type Decision struct {
	Confidence float64 `json:"confidence"`
	Similarity float64 `json:"similarity"`
	Accepted   bool    `json:"accepted"`
	Failed     bool    `json:"failed"`
}

func validConfig(c Config) error {
	if err := validWord(c.Word); err != nil {
		return err
	}
	for _, v := range []float64{c.MinConfidence, c.MinSimilarity} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 1 {
			return fmt.Errorf("判定のしきい値は0より大きく1以下で指定してください")
		}
	}
	return nil
}

// Evaluateは毎回独立に判定する。拒否された発話の内容は保持しない。
func (s *Store) Evaluate(text string, confidence float64, revision uint64, failed bool) Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := Decision{Confidence: confidence, Similarity: Similarity(text, s.config.Word), Failed: failed}
	d.Accepted = s.config.Required && s.revision == revision && !failed && confidence >= s.config.MinConfidence && confidence <= 1 && d.Similarity >= s.config.MinSimilarity
	if s.revision == revision {
		s.last = &d
	}
	return d
}

// Similarityは正規化した文字列の連続部分に対する編集距離を0〜1へ変換する。
func Similarity(text, word string) float64 {
	a, b := []rune(normalize(text)), []rune(normalize(word))
	if len(b) == 0 || len(a) == 0 {
		return 0
	}
	// 長大な文字起こしでも計算量を制限する。
	if len(a) > 2048 {
		a = a[:2048]
	}
	best := 0.0
	for start := range a {
		for length := max(1, len(b)-2); length <= len(b)+2 && start+length <= len(a); length++ {
			x := a[start : start+length]
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
			score := 1 - float64(prev[len(b)])/float64(max(len(x), len(b)))
			if score > best {
				best = score
			}
		}
	}
	return best
}
