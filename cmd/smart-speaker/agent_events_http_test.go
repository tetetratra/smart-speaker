package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/agentbridge"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func TestAgentEventsAPI(t *testing.T) {
	var delivered []types.InboundMessage
	service := agentbridge.NewService(agentbridge.NewClient("http://bridge.invalid", "token"), func(_ context.Context, p types.InboundMessage) error {
		delivered = append(delivered, p)
		return nil
	})
	mux := http.NewServeMux()
	registerAgentEventsAPI(mux, service, "secret-token")
	post := func(token, body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/agent-events", strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	body := `{"type":"completed","event_id":"e1","task_id":"t1","session_id":"s1","request":"読んで","summary":"読みました","details":[]}`
	if code := post("", body); code != http.StatusUnauthorized {
		t.Fatal(code)
	}
	if code := post("wrong", body); code != http.StatusUnauthorized {
		t.Fatal(code)
	}
	if code := post("secret-token", "{"); code != http.StatusBadRequest {
		t.Fatal(code)
	}
	if code := post("secret-token", `{"type":"other","event_id":"e","task_id":"t","session_id":"s"}`); code != http.StatusBadRequest {
		t.Fatal(code)
	}
	for range 2 {
		if code := post("secret-token", body); code != http.StatusAccepted {
			t.Fatal(code)
		}
	}
	if len(delivered) != 1 {
		t.Fatal(delivered)
	}

	disabled := http.NewServeMux()
	registerAgentEventsAPI(disabled, nil, "")
	rec := httptest.NewRecorder()
	disabled.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/agent-events", strings.NewReader(body)))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatal(rec.Code)
	}
}

func TestAgentEventsIngressFailureIsRetryable(t *testing.T) {
	calls := 0
	service := agentbridge.NewService(nil, func(context.Context, types.InboundMessage) error {
		calls++
		if calls == 1 {
			return errors.New("一時的な受付失敗")
		}
		return nil
	})
	mux := http.NewServeMux()
	registerAgentEventsAPI(mux, service, "token")
	for _, want := range []int{http.StatusServiceUnavailable, http.StatusAccepted} {
		req := httptest.NewRequest(http.MethodPost, "/api/agent-events", strings.NewReader(`{"type":"completed","event_id":"e","task_id":"t","session_id":"s"}`))
		req.Header.Set("Authorization", "Bearer token")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatal(rec.Code, rec.Body.String())
		}
	}
}
