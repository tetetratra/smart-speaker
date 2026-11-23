package main

import (
	"context"
	"github.com/tetetratra/smart-speaker/internal/components/automation"
	"github.com/tetetratra/smart-speaker/internal/components/gateway"

	"github.com/tetetratra/smart-speaker/internal/components/tts"
	"github.com/tetetratra/smart-speaker/internal/features/automations"
	"github.com/tetetratra/smart-speaker/internal/features/automations/script"
	"github.com/tetetratra/smart-speaker/internal/features/wakemode"
	"github.com/tetetratra/smart-speaker/internal/plugin"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/tools/catalog"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// runtimeServices はHTTP APIと会話処理で共有する状態をまとめる。
type runtimeServices struct {
	// Plugins は本体に組み込むプラグイン。ツール・HTTP・指示文・購読できる記録をここから集める。
	Plugins    []plugin.Plugin
	AppContext context.Context
	Runner     *script.Runner
	// Tools は会話モデルに渡すツールの一覧。認可などでツールが変わると作り直す。nil なら buildStages で作る。
	Tools       *catalog.Catalog
	WakeMode    *wakemode.Store
	Automations *automations.Service
	// AutomationStage は予約ジョブのコンポーネント。nil ならグラフに載せない。
	AutomationStage *automation.Stage
	// Gateway はグラフの外とグラフの境目。nil なら buildStages で作る。
	Gateway *gateway.Gateway
	// Deliver は Gateway を通して会話へ1件届ける。Claude Codeの通知・Web検索の結果・位置の変化が使う。
	Deliver func(context.Context, types.ConversationInput) error
	TTS     *tts.Selector
	// History は会話履歴。会話管理が記録し、realtime と画面連携が直接読む。nil なら buildStages で作る。
	History *conversationhistory.Store
}
