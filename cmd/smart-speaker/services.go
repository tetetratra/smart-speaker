package main

import (
	"context"

	"github.com/tetetratra/smart-speaker/internal/agentbridge"
	"github.com/tetetratra/smart-speaker/internal/automations"
	"github.com/tetetratra/smart-speaker/internal/automations/script"
	"github.com/tetetratra/smart-speaker/internal/components/tts"
	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// runtimeServices はHTTP APIと会話処理で共有する状態をまとめる。
type runtimeServices struct {
	AppContext   context.Context
	Runner       *script.Runner
	ToolSchemas  []any
	ToolHandlers map[string]tools.Handler
	WakeMode     *wakemode.Store
	Automations  *automations.Service
	Inbound      <-chan types.InboundMessage
	// Deliver は Inbound へ1件送る。予約ジョブ・Claude Codeの通知・Web検索の結果が使う。
	Deliver     func(context.Context, types.InboundMessage) error
	TTS         *tts.Selector
	ToolHistory *toolhistory.Store
	// ClaudeCode はブリッジの設定がないときnil。ツールも通知の受付も無効になる。
	ClaudeCode *agentbridge.Service
}
