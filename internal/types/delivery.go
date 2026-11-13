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
