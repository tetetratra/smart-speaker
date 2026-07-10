package switchbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHub2MeasurementTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1.1/devices/hub-device/status" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": 100,
			"message":    "success",
			"body": map[string]any{
				"temperature": 26.1,
				"humidity":    55,
				"lightLevel":  12,
			},
		})
	}))
	defer server.Close()

	client := newTestClient(server)
	tools := NewHub2ToolsWithClient(client)
	if len(tools) != 3 {
		t.Fatalf("len(tools) = %d, want 3", len(tools))
	}

	tests := []struct {
		name      string
		wantKey   string
		wantValue string
	}{
		{name: hub2GetTemperatureToolName, wantKey: "temperature", wantValue: "26.1"},
		{name: hub2GetHumidityToolName, wantKey: "humidity", wantValue: "55"},
		{name: hub2GetLightLevelToolName, wantKey: "light_level", wantValue: "12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tool *hub2MeasurementTool
			for _, candidate := range tools {
				if candidate.Name() == tt.name {
					tool = candidate
					break
				}
			}
			if tool == nil {
				t.Fatalf("tool %q not found", tt.name)
			}
			tool.SetContext(context.Background())
			out, err := tool.Run(map[string]any{})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if len(out) != 1 {
				t.Fatalf("len(out) = %d, want 1: %#v", len(out), out)
			}
			if got := out[tt.wantKey]; got != tt.wantValue {
				t.Fatalf("%s = %#v, want %q", tt.wantKey, got, tt.wantValue)
			}
		})
	}
}

func TestHub2MeasurementToolReturnsUnavailableWhenFieldMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": 100,
			"message":    "success",
			"body":       map[string]any{},
		})
	}))
	defer server.Close()

	tool := NewHub2ToolsWithClient(newTestClient(server))[0]
	out, err := tool.Run(map[string]any{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := out[tool.spec.resultKey]; got != "取得不可" {
		t.Fatalf("value = %#v, want 取得不可", got)
	}
}

func TestHub2MeasurementToolRequiresClient(t *testing.T) {
	tool := &hub2MeasurementTool{spec: hub2MeasurementSpecs[0]}
	if _, err := tool.Run(map[string]any{}); err != errNotConfigured {
		t.Fatalf("Run() error = %v, want %v", err, errNotConfigured)
	}
}
