// Package plugin は、本体に組み込む機能（プラグイン）と本体との受け口を定める。
//
// プラグインは Plugin を実装し、必要なものだけ ToolProvider などの interface を足す。
// 本体（main）はプラグインを作って並べ、ここの関数でツール・HTTP・指示文・購読できる記録を集める。
package plugin

import (
	"context"
	"net/http"
	"strings"

	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// Deps は、プラグインを作るときに本体から渡すもの。
type Deps struct {
	// AppContext はアプリが動いている間だけ有効な context。
	AppContext context.Context
	// Deliver は、gateway を通して会話へ記録を1件入れる。
	Deliver func(context.Context, types.ConversationInput) error
}

// Plugin は、本体に組み込む機能。
type Plugin interface {
	Name() string
}

// ToolProvider は、AI ツールを持つプラグイン。ツールは tools.DefinitionProvider も実装する。
type ToolProvider interface {
	Tools() []tools.Handler
}

// HTTPRegistrar は、HTTP や WebSocket の受け口を持つプラグイン。自分のパスを mux に登録する。
type HTTPRegistrar interface {
	RegisterHTTP(mux *http.ServeMux)
}

// InstructionProvider は、会話モデルへの指示文を持つプラグイン。システムプロンプトの後ろにつなげる。
type InstructionProvider interface {
	Instructions() string
}

// SourceProvider は、購読できる記録（ConversationInput.Subscribable）を会話へ入れるプラグイン。
// Source と、その記録の説明（予約ジョブのツール説明と画面に出す）を返す。
type SourceProvider interface {
	SubscribableSources() map[string]string
}

// Tools は、プラグインのツールを並び順のまま集める。
func Tools(plugins []Plugin) []tools.Handler {
	var handlers []tools.Handler
	for _, p := range plugins {
		if tp, ok := p.(ToolProvider); ok {
			handlers = append(handlers, tp.Tools()...)
		}
	}
	return handlers
}

// RegisterHTTP は、プラグインの HTTP の受け口を mux に登録する。
func RegisterHTTP(plugins []Plugin, mux *http.ServeMux) {
	for _, p := range plugins {
		if hr, ok := p.(HTTPRegistrar); ok {
			hr.RegisterHTTP(mux)
		}
	}
}

// Instructions は、base の後ろにプラグインの指示文を空行でつなげる。
func Instructions(base string, plugins []Plugin) string {
	var parts []string
	if strings.TrimSpace(base) != "" {
		parts = append(parts, base)
	}
	for _, p := range plugins {
		if ip, ok := p.(InstructionProvider); ok {
			if text := strings.TrimSpace(ip.Instructions()); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// Sources は、プラグインの購読できる記録の Source と説明を集める。
func Sources(plugins []Plugin) map[string]string {
	sources := map[string]string{}
	for _, p := range plugins {
		if sp, ok := p.(SourceProvider); ok {
			for source, description := range sp.SubscribableSources() {
				sources[source] = description
			}
		}
	}
	return sources
}
