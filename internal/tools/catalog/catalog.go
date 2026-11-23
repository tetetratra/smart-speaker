// Package catalog は、会話モデルに渡すツールの一覧（定義・実行処理・指示文）を持ち、動いている間に丸ごと差し替えられるようにする。
//
// MCP サーバーを認可したときなど、起動したあとにツールが変わることがある。
// ツールを使う側（Realtime・toolcaller・予約ジョブ）は、使うたびに Current などで今の一覧を読む。
package catalog

import (
	"sync"
	"sync/atomic"

	"github.com/tetetratra/smart-speaker/internal/tools"
)

// Snapshot は、ある時点のツールの一覧。作ったあとは変えない。
type Snapshot struct {
	// Definitions は会話モデルに渡すツール定義。
	Definitions []any
	// Handlers はツール名から実行処理を引く。
	Handlers map[string]tools.Handler
	// Instructions は会話モデルへの指示文。プラグインの指示文はツールの有無で変わるので、ツールと一緒に作り直す。
	Instructions string
}

// Catalog は、今のツールの一覧を持つ。
type Catalog struct {
	build   func() Snapshot
	current atomic.Pointer[Snapshot]

	// mu は、作り直しを1つずつ進め、listeners を守る。
	mu        sync.Mutex
	listeners []func()
}

// New は、build で一覧を作って持つ。Rebuild のたびに build を呼び直す。
func New(build func() Snapshot) *Catalog {
	c := &Catalog{build: build}
	c.store(build())
	return c
}

// Static は、変わらない一覧を持つ Catalog を作る。テストなどで使う。
func Static(snapshot Snapshot) *Catalog {
	return New(func() Snapshot { return snapshot })
}

func (c *Catalog) store(s Snapshot) {
	if s.Handlers == nil {
		s.Handlers = map[string]tools.Handler{}
	}
	c.current.Store(&s)
}

// Current は今の一覧を返す。返した値は変えないこと。
func (c *Catalog) Current() *Snapshot { return c.current.Load() }

// Definitions は今のツール定義を返す。
func (c *Catalog) Definitions() []any { return c.Current().Definitions }

// Handlers は今の、ツール名から実行処理を引く map を返す。
func (c *Catalog) Handlers() map[string]tools.Handler { return c.Current().Handlers }

// Instructions は今の指示文を返す。
func (c *Catalog) Instructions() string { return c.Current().Instructions }

// Rebuild は一覧を作り直して差し替え、OnChange で登録した関数を呼ぶ。
func (c *Catalog) Rebuild() {
	c.mu.Lock()
	c.store(c.build())
	listeners := append([]func(){}, c.listeners...)
	c.mu.Unlock()
	for _, f := range listeners {
		f()
	}
}

// OnChange は、Rebuild で一覧を差し替えたあとに f を呼ぶよう登録する。
func (c *Catalog) OnChange(f func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.listeners = append(c.listeners, f)
}
