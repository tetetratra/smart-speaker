package tools

import (
	"context"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

const ResponsePolicyKey = "x_response_policy"

// ResponsePolicy はツール結果後の再応答と、応答に添える指示を設定する。
// 失敗時はOnSuccessによらず説明を促す。
type ResponsePolicy struct {
	OnSuccess    bool
	Instructions string
}

func DefinitionWithResponsePolicy(def map[string]any, policy ResponsePolicy) map[string]any {
	def[ResponsePolicyKey] = policy
	return def
}

// ResponsePolicyFromDefinition は未指定なら従来どおり再応答を有効にする。
func ResponsePolicyFromDefinition(def map[string]any) ResponsePolicy {
	if policy, ok := def[ResponsePolicyKey].(ResponsePolicy); ok {
		return policy
	}
	return ResponsePolicy{OnSuccess: true}
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
