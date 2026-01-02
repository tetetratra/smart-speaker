package googlecalendar

import (
	"context"
	"log"

	"smart-speaker/internal/oauth/googlecalendar"
)

const (
	connectorID = "connector_googlecalendar"
)

// Definition はGoogle Calendarコネクタのtools定義を返します。
func Definition() map[string]any {
	def := map[string]any{
		"type":             "mcp",
		"server_label":     "google_calendar",
		"connector_id":     connectorID,
		"require_approval": "never",
	}
	token, err := googlecalendar.AccessToken(context.Background())
	if err != nil {
		log.Printf("googlecalendar: oauth token not available: %v", err)
		return def
	}
	def["authorization"] = token
	return def
}
