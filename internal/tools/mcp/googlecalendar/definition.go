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
	token, err := googlecalendar.AccessToken(context.Background())
	if err != nil {
		log.Printf("googlecalendar: oauth token not available: %v", err)
		return nil
	}
	def := map[string]any{
		"type":             "mcp",
		"server_label":     "google_calendar",
		"connector_id":     connectorID,
		"require_approval": "never",
		"authorization":    token,
	}
	return def
}
