package tools

import (
	"context"
	"strings"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

const (
	ToolModeKey   = "x_tool_mode"
	ToolModeRead  = "read"
	ToolModeWrite = "write"
)

func DefinitionWithMode(def map[string]any, mode string) map[string]any {
	def[ToolModeKey] = mode
	return def
}

func ModeFromDefinition(def map[string]any) string {
	if def == nil {
		return ToolModeRead
	}
	mode, _ := def[ToolModeKey].(string)
	mode = strings.TrimSpace(mode)
	if mode == ToolModeWrite {
		return ToolModeWrite
	}
	return ToolModeRead
}

// Handler はNDJSON tool itemから呼び出されるツールの実体です。
type Handler interface {
	Name() string
	Run(args map[string]any) (map[string]any, error)
}

// ContextAware はtoolcallerのcontextを受け取れるツールが実装します。
type ContextAware interface {
	SetContext(ctx context.Context)
}

// EventEmitterAware は非同期通知用のemitterを受け取れるツールが実装します。
type EventEmitterAware interface {
	SetEventEmitter(func(types.Event))
}

// DefinitionProvider はLLM prompt向けのtool定義を提供します。
type DefinitionProvider interface {
	Definition() map[string]any
}
