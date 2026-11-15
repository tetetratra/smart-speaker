package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/app"
	"github.com/tetetratra/smart-speaker/internal/plugin"
	"github.com/tetetratra/smart-speaker/internal/plugins/userlocation"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// main で組み込んだプラグインのツール・HTTP・購読できる記録が、本体に届く。
func TestPluginsAreWiredIntoApp(t *testing.T) {
	var delivered []types.ConversationInput
	services := runtimeServices{Plugins: buildPlugins(plugin.Deps{Deliver: func(_ context.Context, input types.ConversationInput) error {
		delivered = append(delivered, input)
		return nil
	}, OpenAI: plugin.OpenAIConfig{APIKey: "key", ResponsesModel: "model"}})}
	cfg := app.Config{WebDistDir: t.TempDir()}

	definitions, handlers := buildToolRegistry(cfg, nil, services)
	found := false
	for _, definition := range definitions {
		if definition.(map[string]any)["name"] == "get_current_location" {
			found = true
		}
	}
	if !found || handlers["get_current_location"] == nil {
		t.Fatal("現在地のツールが登録されていません")
	}
	if handlers["web_search"] == nil {
		t.Fatal("Web検索のツールが登録されていません")
	}

	server, chat, err := buildHTTPServer(cfg, nil, services)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeHTTPServer(server); closeStages(chat) })
	rec := httptest.NewRecorder()
	server.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/location", strings.NewReader(`{"place":"自宅"}`)))
	if rec.Code != http.StatusOK || len(delivered) != 1 || delivered[0].Record.Source != userlocation.Source {
		t.Fatalf("位置情報APIが本体の Deliver に届いていません: status=%d delivered=%#v", rec.Code, delivered)
	}

	if _, ok := plugin.Sources(services.Plugins)[userlocation.Source]; !ok {
		t.Fatalf("%s が購読できる記録にありません", userlocation.Source)
	}
}
