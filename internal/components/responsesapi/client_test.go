package responsesapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	types "smart-speaker/internal/types"
)

func TestCreateResponseStream(t *testing.T) {
	t.Run("deltaを改行単位のchunkとして復元する", func(t *testing.T) {
		var gotPayload map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"type\\\":\\\"speech\\\",\\\"text\\\":\\\"こ\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"んにちは\\\"}\\n{\\\"type\\\":\\\"wait\\\",\\\"sec\\\":1}\\n\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"done\"}]}]}}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}))
		defer server.Close()

		client, err := NewClient(Config{APIKey: "test-key", Model: "test-model"})
		if err != nil {
			t.Fatal(err)
		}
		client.endpoint = server.URL

		var lines []string
		resp, err := client.CreateResponseStream(
			context.Background(),
			[]types.ChatMessage{{Role: "user", Content: "こんにちは"}},
			"system",
			func(line string) error {
				lines = append(lines, line)
				return nil
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if stream, ok := gotPayload["stream"].(bool); !ok || !stream {
			t.Fatalf("payload stream = %#v, want true", gotPayload["stream"])
		}
		wantLines := []string{
			`{"type":"speech","text":"こんにちは"}`,
			`{"type":"wait","sec":1}`,
		}
		if len(lines) != len(wantLines) {
			t.Fatalf("lines len = %d, want %d: %#v", len(lines), len(wantLines), lines)
		}
		for i := range wantLines {
			if lines[i] != wantLines[i] {
				t.Fatalf("lines[%d] = %q, want %q", i, lines[i], wantLines[i])
			}
		}
		if resp.ResponseID != "resp_1" {
			t.Fatalf("ResponseID = %q, want resp_1", resp.ResponseID)
		}
	})

	t.Run("stream完了時に残バッファをflushする", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"{\\\"type\\\":\\\"speech\\\",\\\"text\\\":\\\"末尾\\\"}\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_2\",\"output\":[]}}\n\n"))
		}))
		defer server.Close()

		client, err := NewClient(Config{APIKey: "test-key", Model: "test-model"})
		if err != nil {
			t.Fatal(err)
		}
		client.endpoint = server.URL

		var lines []string
		_, err = client.CreateResponseStream(
			context.Background(),
			[]types.ChatMessage{{Role: "user", Content: "こんにちは"}},
			"",
			func(line string) error {
				lines = append(lines, line)
				return nil
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(lines) != 1 || lines[0] != `{"type":"speech","text":"末尾"}` {
			t.Fatalf("lines = %#v", lines)
		}
	})

}
