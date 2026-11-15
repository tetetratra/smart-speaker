// Package userlocation は、ユーザーの居場所を扱うプラグイン。
//
// iPhone のショートカットなどから位置情報 API（/api/location）で場所を受け取り、
// AI ツール get_current_location で返す。場所が変わったときは、購読できる記録として会話へ入れる。
package userlocation

import (
	"context"
	"net/http"

	"github.com/tetetratra/smart-speaker/internal/plugin"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// Source は、場所の変化の記録の Source。予約ジョブの schedule.source に指定できる。
const Source = "user_location"

// Plugin は現在地のプラグイン。居場所はメモリ上だけに持つ。
type Plugin struct {
	store   *Store
	deliver func(context.Context, types.ConversationInput) error
}

// New は現在地のプラグインを作る。deps.Deliver が nil なら、場所の変化を会話へ入れない。
func New(deps plugin.Deps) *Plugin {
	return &Plugin{store: &Store{}, deliver: deps.Deliver}
}

func (p *Plugin) Name() string { return "userlocation" }

func (p *Plugin) Tools() []tools.Handler {
	return []tools.Handler{&Tool{Store: p.store}}
}

func (p *Plugin) RegisterHTTP(mux *http.ServeMux) {
	registerLocationAPI(mux, p.store, p.deliver)
}

func (p *Plugin) SubscribableSources() map[string]string {
	return map[string]string{
		Source: "位置情報APIで受け取った場所が変わった（event.metadata.place に新しい場所、event.metadata.previous_place に前の場所。サーバー起動後の最初の1件は null）",
	}
}
