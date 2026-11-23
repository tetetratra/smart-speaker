package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/tools"
)

type fullPlugin struct{}

func (fullPlugin) Name() string           { return "full" }
func (fullPlugin) Tools() []tools.Handler { return []tools.Handler{namedTool("a"), namedTool("b")} }
func (fullPlugin) RegisterHTTP(mux *http.ServeMux) {
	mux.HandleFunc("/full", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
}
func (fullPlugin) Instructions() string                   { return " 指示 \n" }
func (fullPlugin) SubscribableSources() map[string]string { return map[string]string{"full": "説明"} }

type emptyPlugin struct{}

func (emptyPlugin) Name() string { return "empty" }

type namedTool string

func (t namedTool) Name() string { return string(t) }
func (namedTool) Run(context.Context, map[string]any) (map[string]any, error) {
	return nil, nil
}

// プラグインが実装した interface の分だけ集め、実装していないものは飛ばす。
func TestCollectors(t *testing.T) {
	plugins := []Plugin{emptyPlugin{}, fullPlugin{}}

	got := Tools(plugins)
	if len(got) != 2 || got[0].Name() != "a" || got[1].Name() != "b" {
		t.Fatalf("Tools = %#v", got)
	}

	mux := http.NewServeMux()
	RegisterHTTP(plugins, mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/full", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("HTTP が登録されていません: %d", rec.Code)
	}

	if got := Instructions("本体", plugins); got != "本体\n\n指示" {
		t.Fatalf("Instructions = %q", got)
	}
	if got := Instructions("本体", []Plugin{emptyPlugin{}}); got != "本体" {
		t.Fatalf("指示文のないプラグインで変わった: %q", got)
	}

	if got := Sources(plugins); len(got) != 1 || got["full"] != "説明" {
		t.Fatalf("Sources = %#v", got)
	}
}

func TestUnknownNames(t *testing.T) {
	got := UnknownNames([]Plugin{emptyPlugin{}, fullPlugin{}}, []string{"empty", "fulll", "full", "other"})
	if len(got) != 2 || got[0] != "fulll" || got[1] != "other" {
		t.Fatalf("UnknownNames = %v", got)
	}
}

func TestDecodeConfigWithoutConfig(t *testing.T) {
	v := "既定値"
	if err := (Deps{}).DecodeConfig("full", &v); err != nil || v != "既定値" {
		t.Fatalf("v = %q, err = %v", v, err)
	}
	var gotName string
	deps := Deps{Config: func(name string, _ any) error { gotName = name; return nil }}
	if err := deps.DecodeConfig("full", &v); err != nil || gotName != "full" {
		t.Fatalf("name = %q, err = %v", gotName, err)
	}
}

type notifyingPlugin struct{ listeners *[]func() }

func (notifyingPlugin) Name() string              { return "notifying" }
func (p notifyingPlugin) OnToolsChanged(f func()) { *p.listeners = append(*p.listeners, f) }

// ツールが変わることがあるプラグインにだけ、変わったときの関数を登録する。
func TestOnToolsChanged(t *testing.T) {
	var listeners []func()
	called := 0
	OnToolsChanged([]Plugin{emptyPlugin{}, fullPlugin{}, notifyingPlugin{&listeners}}, func() { called++ })
	if len(listeners) != 1 {
		t.Fatalf("登録した数 = %d", len(listeners))
	}
	listeners[0]()
	if called != 1 {
		t.Fatalf("called = %d", called)
	}
}
