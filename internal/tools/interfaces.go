package tools

import (
	"context"
)

// Handler はNDJSON tool itemから呼び出されるツールの実体です。
type Handler interface {
	Name() string
	Run(args map[string]any) (map[string]any, error)
}

// ContextAware はtoolcallerのcontextを受け取れるツールが実装します。
type ContextAware interface {
	SetContext(ctx context.Context)
}

// DefinitionProvider はLLM prompt向けのtool定義を提供します。
type DefinitionProvider interface {
	Definition() map[string]any
}
