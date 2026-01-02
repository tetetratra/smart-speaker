package tools

import (
	"context"

	types "smart-speaker/internal/types"
)

// Handler はfunction callingから呼び出されるツールの実体です。
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

// DefinitionProvider はResponses API向けのtools定義を提供します。
type DefinitionProvider interface {
	Definition() map[string]any
}
