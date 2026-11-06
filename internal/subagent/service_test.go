package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClient struct{ calls atomic.Int32 }

func (c *fakeClient) Respond(context.Context, string, string) (Answer, error) {
	n := c.calls.Add(1)
	return Answer{ID: fmt.Sprint(n), Text: "回答"}, nil
}
func pending(t *testing.T, s *Service) Request {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("許可要求が作られませんでした")
		default:
		}
		if requests := s.Pending(); len(requests) > 0 {
			return requests[0]
		}
		time.Sleep(time.Millisecond)
	}
}
func TestNoModelCallWithoutPerRequestApproval(t *testing.T) {
	client := &fakeClient{}
	s := New(client)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	denied := make(chan error, 1)
	go func() { _, err := s.Call(ctx, "複雑な質問", ""); denied <- err }()
	req := pending(t, s)
	if client.calls.Load() != 0 {
		t.Fatal("許可前にモデルを呼び出しました")
	}
	if err := s.Resolve(req.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := <-denied; err == nil {
		t.Fatal("拒否が反映されませんでした")
	}
	if client.calls.Load() != 0 {
		t.Fatal("拒否後にモデルを呼び出しました")
	}
	if err := s.Resolve(req.ID, true); err == nil {
		t.Fatal("古い許可要求を再利用できました")
	}
	first := make(chan Result, 1)
	go func() {
		r, err := s.Call(ctx, "初回", "")
		if err != nil {
			denied <- err
			return
		}
		first <- r
	}()
	req = pending(t, s)
	_ = s.Resolve(req.ID, true)
	var result Result
	select {
	case result = <-first:
	case err := <-denied:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if result.SessionID == "" || result.Text != "回答" || client.calls.Load() != 1 {
		t.Fatal(result, client.calls.Load())
	}
	go func() { _, err := s.Call(ctx, "続き", result.SessionID); denied <- err }()
	req = pending(t, s)
	if client.calls.Load() != 1 {
		t.Fatal("継続時の許可を省略しました")
	}
	_ = s.Resolve(req.ID, false)
	if err := <-denied; err == nil {
		t.Fatal("継続時の拒否が反映されませんでした")
	}
}
func TestApprovalTimeoutCancellationAndUnknownSession(t *testing.T) {
	client := &fakeClient{}
	s := New(client)
	s.ApprovalTimeout = 5 * time.Millisecond
	if _, err := s.Call(context.Background(), "依頼", ""); err == nil {
		t.Fatal("期限切れが成功しました")
	}
	if len(s.Pending()) != 0 || client.calls.Load() != 0 {
		t.Fatal("期限切れ後に要求が残りました")
	}
	if _, err := s.Call(context.Background(), "依頼", "missing"); err == nil {
		t.Fatal("不明なIDが受理されました")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Call(ctx, "依頼", ""); err == nil {
		t.Fatal("中止が成功しました")
	}
	if len(s.Pending()) != 0 || client.calls.Load() != 0 {
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
		_ = json.NewEncoder(w).Encode(map[string]any{"id": fmt.Sprintf("resp-%d", n), "status": "completed", "output": []any{map[string]any{"type": "reasoning"}, map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": "回答の全文"}}}}})
	}))
	defer api.Close()
	s := New(&OpenAIClient{APIKey: "test-key", Model: "test-model", Endpoint: api.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var id string
	for n := 1; n <= 3; n++ {
		done := make(chan struct{})
		var result Result
		var err error
		go func() { result, err = s.Call(ctx, fmt.Sprintf("依頼%d", n), id); close(done) }()
		req := pending(t, s)
		if req.Input != fmt.Sprintf("依頼%d", n) {
			t.Fatal(req)
		}
		_ = s.Resolve(req.ID, true)
		<-done
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
		if p["instructions"] == nil || p["tools"] != nil {
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
