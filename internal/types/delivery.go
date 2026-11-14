package types

import (
	"encoding/json"
	"fmt"
)

const MaxDeliveryContextBytes = 16 * 1024

// ValidateDeliveryContext は本体で解釈する通知contextを検証する。未知の項目は保持する。
func ValidateDeliveryContext(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	if len(raw) > MaxDeliveryContextBytes {
		return fmt.Errorf("contextは16KiB以内で指定してください")
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return fmt.Errorf("contextはJSONオブジェクトで指定してください")
	}
	if delivery, ok := value["delivery"]; ok {
		var mode string
		if json.Unmarshal(delivery, &mode) != nil || mode != "history_only" {
			return fmt.Errorf("未対応のdeliveryです")
		}
	}
	return nil
}

// HistoryOnly は通知を履歴に記録するだけかを返す。受信境界で検証してから使う。
func HistoryOnly(raw json.RawMessage) bool {
	var value struct {
		Delivery string `json:"delivery"`
	}
	return json.Unmarshal(raw, &value) == nil && value.Delivery == "history_only"
}

// ExternalInput は、会話の外から届いた入力を、会話管理へ渡す形にする。
// deliveryContext はツールの実行や agent-bridge が運ぶ通知の context（JSON）。
// history_only なら情報追加、それ以外は応答依頼にする。delivery は制御情報なので履歴には残さず、
// ほかの項目（予約ジョブの識別情報など）は記録の付随情報として残す。
func ExternalInput(record ConversationRecord, deliveryContext json.RawMessage) ConversationInput {
	handling := ModelHandlingAddAndRespond
	if HistoryOnly(deliveryContext) {
		handling = ModelHandlingAddToContext
	}
	var metadata map[string]json.RawMessage
	_ = json.Unmarshal(deliveryContext, &metadata)
	delete(metadata, "delivery")
	if len(metadata) > 0 {
		raw, _ := json.Marshal(metadata)
		record.Metadata = map[string]any{"context": string(raw)}
	}
	if record.Role == "" {
		record.Role = RoleSystem
	}
	record.External = true
	return ConversationInput{Record: record, ModelHandling: handling}
}
