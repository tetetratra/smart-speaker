package main

import (
	"github.com/tetetratra/smart-speaker/internal/agentbridge"
	"github.com/tetetratra/smart-speaker/internal/automations"
	"github.com/tetetratra/smart-speaker/internal/components/tts"
	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// runtimeServices はHTTP APIと会話処理で共有する状態をまとめる。
type runtimeServices struct {
	WakeMode    *wakemode.Store
	Automations *automations.Service
	Scheduled   <-chan types.ScheduledPrompt
	// DirectToolCalls は予約ジョブがAIを通さずに呼んだツールを会話履歴へ送る。
	DirectToolCalls <-chan types.DirectToolCall
	// AutomationRunner は予約ジョブのスクリプトを実行する。
	AutomationRunner *automationRunner
	TTS              *tts.Selector
	ToolHistory      *toolhistory.Store
	// ClaudeCode はブリッジの設定がないときnil。ツールも通知の受付も無効になる。
	ClaudeCode *agentbridge.Service
}
