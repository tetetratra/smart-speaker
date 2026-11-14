// Package playbackvolumeは、接続中の全端末で共通の再生音量を保持する。
// 永続化はせず、サーバーを再起動すると既定値に戻る。
package playbackvolume

import "sync"

// Levelsは小さい順の音量の段階。倍率は画面側で音声に掛ける。
var Levels = []string{"mute", "quiet", "low", "normal", "boost"}

// Defaultは起動直後の音量。画面の既定値と揃える。
const Default = "boost"

// Storeはゼロ値で既定の音量を持つ。
type Store struct {
	mu          sync.Mutex
	level       string
	subscribers map[chan struct{}]struct{}
}

// Valid は level が音量の段階として正しいかを返す。
func Valid(level string) bool {
	return indexOf(level) >= 0
}

func (s *Store) Level() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.levelLocked()
}

// Setは音量を変え、変更前の音量を返す。段階として正しくない値は受け付けない。
func (s *Store) Set(level string) (previous string, ok bool) {
	if !Valid(level) {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous = s.levelLocked()
	s.level = level
	if previous != level {
		s.notifyLocked()
	}
	return previous, true
}

// Stepは現在の音量からdelta段階だけ動かし、変更前後の音量を返す。端では止まる。
func (s *Store) Step(delta int) (previous, current string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous = s.levelLocked()
	i := max(0, min(len(Levels)-1, indexOf(previous)+delta))
	s.level = Levels[i]
	if previous != s.level {
		s.notifyLocked()
	}
	return previous, s.level
}

// Subscribeは音量の変更を通知する。購読後にLevelを読むことで初期取得中の変更も取りこぼさない。
// 通知が重なった場合はまとめ、受信側で最新の音量を取得する。
func (s *Store) Subscribe() (<-chan struct{}, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subscribers == nil {
		s.subscribers = make(map[chan struct{}]struct{})
	}
	changed := make(chan struct{}, 1)
	s.subscribers[changed] = struct{}{}
	return changed, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.subscribers, changed)
	}
}

func (s *Store) levelLocked() string {
	if s.level == "" {
		return Default
	}
	return s.level
}

func (s *Store) notifyLocked() {
	for changed := range s.subscribers {
		select {
		case changed <- struct{}{}:
		default:
		}
	}
}

func indexOf(level string) int {
	for i, l := range Levels {
		if l == level {
			return i
		}
	}
	return -1
}
