package automations

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/automations/script"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func TestScriptExamplesRunWithDocumentedToolResults(t *testing.T) {
	for _, tt := range []struct {
		name, source, temperature string
		tools                     []string
		reminders                 int
	}{
		{"直接実行", directScriptExample, "", []string{"set_whiteboard"}, 0},
		{"リマインダー", remindScriptExample, "", nil, 1},
		{"28度以上", temperatureScriptExample, "28", []string{"hub2_get_temperature", "set_whiteboard"}, 1},
		{"28度未満", temperatureScriptExample, "27.9", []string{"hub2_get_temperature"}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, operation := range []string{"register_job", "update_job"} {
				description := (&Tool{Operation: operation}).Definition()["description"].(string)
				if !strings.Contains(description, tt.source) {
					t.Fatal("実行した例がツール説明にありません", operation)
				}
			}
			var called []string
			reminders := 0
			runner := script.New(func(_ context.Context, req types.ToolRequest) types.ToolResultRecord {
				called = append(called, req.Name)
				var result map[string]any
				switch req.Name {
				case "hub2_get_temperature":
					result = map[string]any{"temperature": tt.temperature}
				case "set_whiteboard":
					result = map[string]any{"updated": true}
				default:
					t.Fatal("例が未定義ツールを呼びました", req.Name)
				}
				output, _ := json.Marshal(result)
				return types.ToolResultRecord{Output: output}
			}, func(_ context.Context, message types.InboundMessage) error {
				if !types.HistoryOnly(message.Context) {
					reminders++
				}
				return nil
			})
			if err := runner.Run(context.Background(), "example", "説明例", tt.source); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(called, tt.tools) || reminders != tt.reminders {
				t.Fatalf("呼出し=%v 通知=%d", called, reminders)
			}
		})
	}
}
