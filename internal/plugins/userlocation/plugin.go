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

// Instructions は、居場所の変化の通知と、現在値の取得結果の読み方を伝える。
func (p *Plugin) Instructions() string {
	return `## ユーザーの居場所の状態通知
- システムは、位置情報APIで最初に場所を受け取ったときと、受け取った場所が前回から変わったときに、user_locationのイベントを会話へ届けます。同じ場所の更新ではイベントを届けません。
- 初回の通知後に新しい位置情報イベントが届いていない場合は、システムが把握する居場所が前回から変わっていないという意味です。最後に届いた場所を引き続き使い、イベントが新しくないことだけを理由に不明としたり、現在地を保証できないという留保を繰り返したりしないでください。この扱いは居場所の状態通知に限り、イベント全般には適用しないでください。
- 現在の居場所を確かめるときはget_current_locationを使い、その取得結果を過去のイベントより優先してください。locationがnullなら現在は未設定なので、過去のイベントやメモリの場所で埋めず、未設定と伝えてください。
- 例：最後の通知が「外出中から自宅に変わりました」で、その後の変更通知がなければ、システムが把握する居場所は自宅です。これは端末のGPSをその場で測定したという意味ではありません。`
}

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
