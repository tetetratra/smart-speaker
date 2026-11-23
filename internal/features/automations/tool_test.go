package automations

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/features/automations/script"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func TestScriptExamplesRunWithDocumentedToolResults(t *testing.T) {
	for _, tt := range []struct {
		name, source, event string
		tools               []string
		reminders           int
	}{
		{"直接実行", directScriptExample, "", []string{"set_whiteboard"}, 0},
		{"リマインダー", remindScriptExample, "", nil, 1},
		{"28度以上", temperatureScriptExample, `{"metadata":{"temperature":28}}`, []string{"set_whiteboard"}, 1},
		{"28度未満", temperatureScriptExample, `{"metadata":{"temperature":27.9}}`, nil, 0},
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
				case "set_whiteboard":
					result = map[string]any{"updated": true}
				default:
					t.Fatal("例が未定義ツールを呼びました", req.Name)
				}
				output, _ := json.Marshal(result)
				return types.ToolResultRecord{Output: output}
			}, func(_ context.Context, message types.ConversationInput) error {
				if message.ModelShouldRespond {
					reminders++
				}
				return nil
			})
			if err := runner.RunWithEvent(context.Background(), "example", "説明例", tt.source, json.RawMessage(tt.event)); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(called, tt.tools) || reminders != tt.reminders {
				t.Fatalf("呼出し=%v 通知=%d", called, reminders)
			}
		})
	}
}
