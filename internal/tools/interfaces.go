package tools

import (
	"context"
)

// Handler はNDJSON tool itemから呼び出されるツールの実体です。
type Handler interface {
	Name() string
	Run(ctx context.Context, args map[string]any) (map[string]any, error)
}

// DefinitionProvider はLLM prompt向けのtool定義を提供します。
type DefinitionProvider interface {
	Definition() map[string]any
}

// TitleProvider は画面の履歴に添える任意のタイトルを提供します。
type TitleProvider interface {
	Title(args map[string]any) string
}
