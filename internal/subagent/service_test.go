package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClient struct{ calls atomic.Int32 }

func (c *fakeClient) Respond(context.Context, string, string) (Answer, error) {
	n := c.calls.Add(1)
	return Answer{ID: fmt.Sprint(n), Summary: "要点", Details: []string{"補足"}}, nil
}
func TestDirectCallWithoutPermissionGate(t *testing.T) {
	client := &fakeClient{}
	s := New(client)
	first, err := s.Call(context.Background(), "複雑な質問", "")
	if err != nil || first.SessionID == "" || first.Summary != "要点" || client.calls.Load() != 1 {
		t.Fatal(first, err)
	}
	second, err := s.Call(context.Background(), "続き", first.SessionID)
	if err != nil || second.SessionID != first.SessionID || client.calls.Load() != 2 {
		t.Fatal(second, err)
	}
	if _, err := s.Call(context.Background(), "依頼", "missing"); err == nil {
		t.Fatal("不明なIDが受理されました")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Call(ctx, "依頼", ""); err == nil || client.calls.Load() != 2 {
		t.Fatal("中止後に送信されました")
	}
}
func TestResponsesConversationChainingAndFailurePreservesPriorID(t *testing.T) {
	var mu sync.Mutex
	var payloads []map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("認証がありません")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		payloads = append(payloads, body)
		n := len(payloads)
		mu.Unlock()
		if n == 2 {
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":"失敗"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": fmt.Sprintf("resp-%d", n), "status": "completed", "output": []any{map[string]any{"type": "reasoning"}, map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": `{"summary":"短い要点","details":["補足1","補足2"]}`}}}}})
	}))
	defer api.Close()
	s := New(&OpenAIClient{APIKey: "test-key", Model: "test-model", Endpoint: api.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var id string
	for n := 1; n <= 3; n++ {
		result, err := s.Call(ctx, fmt.Sprintf("依頼%d", n), id)
		if n == 2 {
			if err == nil {
				t.Fatal("APIエラーが成功しました")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if id != "" && id != result.SessionID {
			t.Fatal("継続でIDが変わりました")
		}
		id = result.SessionID
	}
	mu.Lock()
	defer mu.Unlock()
	if _, ok := payloads[0]["previous_response_id"]; ok {
		t.Fatal("初回に履歴を引き継ぎました")
	}
	if payloads[1]["previous_response_id"] != "resp-1" || payloads[2]["previous_response_id"] != "resp-1" {
		t.Fatal(payloads)
	}
	for _, p := range payloads {
		if !strings.Contains(p["instructions"].(string), "サブエージェント") || !strings.Contains(p["instructions"].(string), "仲介") || p["text"] == nil || p["tools"] != nil {
			t.Fatal("テキスト専用の設定が不正です")
		}
	}
	s.mu.Lock()
	s.sessions[id].touched = time.Now().Add(-25 * time.Hour)
	s.mu.Unlock()
	if _, err := s.Call(ctx, "期限切れ", id); err == nil {
		t.Fatal("期限切れセッションが再開されました")
	}
}

func TestRejectLongOrMalformedAnswer(t *testing.T) {
	for _, answer := range []any{
		map[string]any{"summary": strings.Repeat("長", 201), "details": []string{}},
		map[string]any{"summary": "要点", "details": []string{"複数\n行"}},
		map[string]any{"summary": "要点", "details": make([]string, 11)},
	} {
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			text, _ := json.Marshal(answer)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "r1", "status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": string(text)}}}}})
		}))
		client := &OpenAIClient{Endpoint: api.URL}
		if _, err := client.Respond(context.Background(), "質問", ""); err == nil {
			t.Error("不正な回答を受理しました")
		}
		api.Close()
	}
}
