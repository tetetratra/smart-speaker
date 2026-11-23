package switchbot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookManagementRequests(t *testing.T) {
	var got []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		assertValidSwitchBotAuth(t, r)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		got = append(got, body)
		switch r.URL.Path {
		case "/v1.1/webhook/queryWebhook":
			_, _ = w.Write([]byte(`{"statusCode":100,"body":{"urls":["https://ours"]}}`))
		case "/v1.1/webhook/setupWebhook", "/v1.1/webhook/deleteWebhook":
			_, _ = w.Write([]byte(`{"statusCode":100,"body":{}}`))
		default:
			t.Errorf("path=%s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := newTestClient(server)
	ctx := context.Background()
	urls, err := client.WebhookURLs(ctx)
	if err != nil || len(urls) != 1 || urls[0] != "https://ours" {
		t.Fatalf("urls=%v err=%v", urls, err)
	}
	if err := client.RegisterWebhook(ctx, "https://ours"); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteWebhook(ctx, "https://ours"); err != nil {
		t.Fatal(err)
	}
	if got[0]["action"] != "queryUrl" || got[1]["action"] != "setupWebhook" || got[1]["deviceList"] != "ALL" || got[1]["url"] != "https://ours" || got[2]["action"] != "deleteWebhook" || got[2]["url"] != "https://ours" {
		t.Fatalf("payload=%#v", got)
	}
}

func TestWebhookManagementReturnsAPIFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"statusCode":190,"message":"error"}`))
	}))
	defer server.Close()
	client := newTestClient(server)
	ctx := context.Background()
	if _, err := client.WebhookURLs(ctx); err == nil {
		t.Fatal("照会の失敗が返りません")
	}
	if err := client.RegisterWebhook(ctx, "https://ours"); err == nil {
		t.Fatal("登録の失敗が返りません")
	}
	if err := client.DeleteWebhook(ctx, "https://ours"); err == nil {
		t.Fatal("解除の失敗が返りません")
	}
}

func TestWebhookQueryRejectsMissingURLs(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"urls":null}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"statusCode":100,"body":` + body + `}`))
			}))
			defer server.Close()
			if _, err := newTestClient(server).WebhookURLs(context.Background()); err == nil {
				t.Fatal("送信先を確認できない応答を受理しました")
			}
		})
	}
}
