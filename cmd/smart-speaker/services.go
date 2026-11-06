package main

import (
	"github.com/tetetratra/smart-speaker/internal/components/tts"
	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/subagent"
)

// runtimeServices はHTTP APIと会話処理で共有する状態をまとめる。
type runtimeServices struct {
	TTS         *tts.Selector
	ToolHistory *toolhistory.Store
	Subagent    *subagent.Service
}
