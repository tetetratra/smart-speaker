package execution

import (
	"context"
	"encoding/json"
)

type deliveryContextKey struct{}

// WithDeliveryContext は呼び出し固有の通知contextを複製して保持する。
func WithDeliveryContext(ctx context.Context, raw json.RawMessage) context.Context {
	return context.WithValue(ctx, deliveryContextKey{}, append(json.RawMessage(nil), raw...))
}

// DeliveryContext は後続通知へ引き継ぐcontextの複製を返す。
func DeliveryContext(ctx context.Context) json.RawMessage {
	raw, _ := ctx.Value(deliveryContextKey{}).(json.RawMessage)
	return append(json.RawMessage(nil), raw...)
}
