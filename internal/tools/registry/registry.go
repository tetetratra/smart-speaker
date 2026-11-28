package registry

import (
	"context"

	"github.com/tetetratra/smart-speaker/internal/features/automations"
	"github.com/tetetratra/smart-speaker/internal/features/memory"
	"github.com/tetetratra/smart-speaker/internal/features/wakemode"
	"github.com/tetetratra/smart-speaker/internal/features/whiteboard"
	"github.com/tetetratra/smart-speaker/internal/tools"
)

// Registry はツール定義とハンドラをまとめて管理します。
type Registry struct {
	entries []entry
}

type entry struct {
	def     map[string]any
	handler tools.Handler
}

// Config はツールの生成に必要な設定を持ちます。
type Config struct {
	AppContext  context.Context
	WakeMode    *wakemode.Store
	Automations *automations.Service
	Memory      *memory.Store
	// PluginTools はプラグインのツール。tools.DefinitionProvider を実装しないものは登録しない。
	PluginTools []tools.Handler
}

// New は利用可能なツールをまとめて登録します。
func New(cfg Config) *Registry {
	var entries []entry

	whiteboardTool := whiteboard.New()
	toolEntries := []entry{
		{def: whiteboardTool.Definition(), handler: whiteboardTool},
	}
	if cfg.WakeMode != nil {
		for _, readOnly := range []bool{true, false} {
			t := &wakemode.Tool{Store: cfg.WakeMode, ReadOnly: readOnly}
			toolEntries = append(toolEntries, entry{def: t.Definition(), handler: t})
		}
	}
	if cfg.Automations != nil {
		for _, operation := range []string{"register_job", "list_jobs", "update_job", "delete_job"} {
			t := &automations.Tool{Service: cfg.Automations, Operation: operation}
			toolEntries = append(toolEntries, entry{def: t.Definition(), handler: t})
		}
	}
	if cfg.Memory != nil {
		for _, operation := range memory.Operations {
			t := &memory.Tool{Store: cfg.Memory, Operation: operation}
			toolEntries = append(toolEntries, entry{def: t.Definition(), handler: t})
		}
	}

	for _, t := range cfg.PluginTools {
		if dp, ok := t.(tools.DefinitionProvider); ok {
			toolEntries = append(toolEntries, entry{def: dp.Definition(), handler: t})
		}
	}
	for _, e := range toolEntries {
		entries = append(entries, e)
	}

	return &Registry{entries: entries}
}

// Handlers はtoolcaller向けのname->handlerマップを返します。
func (r *Registry) Handlers() map[string]tools.Handler {
	handlers := map[string]tools.Handler{}
	if r == nil {
		return handlers
	}
	for _, e := range r.entries {
		if e.handler == nil {
			continue
		}
		handlers[e.handler.Name()] = e.handler
	}
	return handlers
}

// Definitions は全ツールをfunction calling用に返す（頭脳AIに渡す）。
func (r *Registry) Definitions() []any {
	defs := make([]any, 0, len(r.entries))
	for _, e := range r.entries {
		if e.def == nil {
			continue
		}
		def := map[string]any{}
		for _, key := range []string{"type", "name", "description", "parameters"} {
			if value, ok := e.def[key]; ok {
				def[key] = value
			}
		}
		defs = append(defs, def)
	}
	return defs
}
