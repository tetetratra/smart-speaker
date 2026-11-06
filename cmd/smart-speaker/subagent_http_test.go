package main

import (
	"context"
	"github.com/tetetratra/smart-speaker/internal/subagent"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type approvedClient struct{ calls atomic.Int32 }

func (c *approvedClient) Respond(context.Context, string, string) (subagent.Answer, error) {
	c.calls.Add(1)
	return subagent.Answer{ID: "response-id", Text: "回答"}, nil
}
func TestSubagentApprovalHTTPIsRequiredAndCannotBeReused(t *testing.T) {
	client := &approvedClient{}
	service := subagent.New(client)
	mux := http.NewServeMux()
	registerSubagentAPI(mux, service)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := service.Call(ctx, "送信してよい依頼文", ""); done <- err }()
	var id string
	for id == "" {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		default:
		}
		if p := service.Pending(); len(p) > 0 {
			id = p[0].ID
		} else {
			time.Sleep(time.Millisecond)
		}
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/subagent/permissions", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "送信してよい依頼文") || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(rec.Code, rec.Body)
	}
	for _, body := range []string{`{}`, `{"approved":"true"}`, `{"approved":true} {}`, `{"approved":true,"extra":1}`} {
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/subagent/permissions/"+id, strings.NewReader(body)))
		if rec.Code != 400 || client.calls.Load() != 0 {
			t.Fatal(rec.Code, "許可前の送信が発生しました")
		}
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/subagent/permissions/"+id, strings.NewReader(`{"approved":true}`)))
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if client.calls.Load() != 1 {
		t.Fatal("許可後に実行されませんでした")
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/subagent/permissions/"+id, strings.NewReader(`{"approved":true}`)))
	if rec.Code != 409 {
		t.Fatal("許可を再利用できました", rec.Code)
	}
}
