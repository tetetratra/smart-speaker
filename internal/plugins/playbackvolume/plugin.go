// Package playbackvolume は、接続中の全端末で共通の再生音量を扱うプラグイン。
//
// AI ツール set_playback_volume と、画面との同期用の WebSocket（/ws/playback-volume）を持つ。
// 倍率は画面側で音声に掛ける（web/src/plugins/playbackvolume）。
package playbackvolume

import (
	"net/http"

	"github.com/tetetratra/smart-speaker/internal/plugin"
	"github.com/tetetratra/smart-speaker/internal/tools"
)

// Plugin は再生音量のプラグイン。
type Plugin struct {
	store *Store
}

// New は再生音量のプラグインを作る。
func New(plugin.Deps) *Plugin {
	return &Plugin{store: &Store{}}
}

func (p *Plugin) Name() string { return "playbackvolume" }

func (p *Plugin) Tools() []tools.Handler {
	return []tools.Handler{&Tool{Store: p.store}}
}

func (p *Plugin) RegisterHTTP(mux *http.ServeMux) {
	registerWebSocket(mux, p.store)
}
