package main

import (
	"github.com/tetetratra/smart-speaker/internal/automations"
	"github.com/tetetratra/smart-speaker/internal/components/tts"
	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// runtimeServices はHTTP APIと会話処理で共有する状態をまとめる。
type runtimeServices struct {
	Automations *automations.Service
	Scheduled   <-chan types.ScheduledPrompt
	TTS         *tts.Selector
	ToolHistory *toolhistory.Store
}
