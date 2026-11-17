package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tetetratra/smart-speaker/internal/plugin"
	"github.com/tetetratra/smart-speaker/internal/tools"
)

// stdioEnv がある場合、テストのバイナリは stdio の MCP サーバーとして動く。stdio のサーバーのテストで子プロセスとして起動する。
const stdioEnv = "MCP_PLUGIN_TEST_STDIO_SERVER"

func TestMain(m *testing.M) {
	if os.Getenv(stdioEnv) != "" {
		if err := newTestMCPServer().Run(context.Background(), &sdk.StdioTransport{}); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type echoInput struct {
	Text string `json:"text" jsonschema:"返す文字列"`
}

// newTestMCPServer は、echo・fail・hidden のツールを持つ MCP サーバーを作る。
func newTestMCPServer() *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "1.0.0"}, nil)
	sdk.AddTool(server, &sdk.Tool{Name: "echo", Description: "文字列をそのまま返す"}, func(_ context.Context, _ *sdk.CallToolRequest, in echoInput) (*sdk.CallToolResult, any, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "echo: " + in.Text + " " + os.Getenv("MCP_TEST_SUFFIX")}}}, nil, nil
	})
	sdk.AddTool(server, &sdk.Tool{Name: "fail", Description: "必ず失敗する"}, func(context.Context, *sdk.CallToolRequest, struct{}) (*sdk.CallToolResult, any, error) {
		return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: "壊れています"}}}, nil, nil
	})
	sdk.AddTool(server, &sdk.Tool{Name: "hidden", Description: "allowed_tools で隠す"}, func(context.Context, *sdk.CallToolRequest, struct{}) (*sdk.CallToolResult, any, error) {
		return &sdk.CallToolResult{}, nil, nil
	})
	return server
}

func newTestHTTPServer(t *testing.T, check func(*http.Request) bool) *httptest.Server {
	t.Helper()
	server := newTestMCPServer()
	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if check != nil && !check(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	// 待ち受け中の SSE の接続が残ると Close が待ち続けるので、先に切る。
	t.Cleanup(func() { ts.CloseClientConnections(); ts.Close() })
	return ts
}

func newPlugin(t *testing.T, cfg Config) *Plugin {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	p, err := New(plugin.Deps{AppContext: ctx, DataDir: t.TempDir(), Config: func(_ string, v any) error {
		*(v.(*Config)) = cfg
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func toolByName(t *testing.T, p *Plugin, name string) tools.Handler {
	t.Helper()
	for _, h := range p.Tools() {
		if h.Name() == name {
			return h
		}
	}
	t.Fatalf("ツール %s がありません", name)
	return nil
}

func toolNames(p *Plugin) []string {
	var names []string
	for _, h := range p.Tools() {
		names = append(names, h.Name())
	}
	return names
}

func TestRemoteServerTools(t *testing.T) {
	ts := newTestHTTPServer(t, func(r *http.Request) bool { return r.Header.Get("X-Token") == "secret-value" })
	t.Setenv("TEST_MCP_TOKEN", "secret-value")
	p := newPlugin(t, Config{Servers: map[string]ServerConfig{
		"remote": {URL: ts.URL, Headers: map[string]string{"X-Token": "${TEST_MCP_TOKEN}"}, AllowedTools: []string{"echo", "fail"}},
	}})

	if got := strings.Join(toolNames(p), ","); got != "remote__echo,remote__fail" {
		t.Fatalf("tools = %s", got)
	}
	echo := toolByName(t, p, "remote__echo")
	def := echo.(tools.DefinitionProvider).Definition()
	params := def["parameters"].(map[string]any)
	if params["type"] != "object" || params["properties"].(map[string]any)["text"] == nil {
		t.Fatalf("parameters = %#v", params)
	}
	out, err := echo.Run(context.Background(), map[string]any{"text": "やあ"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out["result"].(string), "echo: やあ") {
		t.Fatalf("result = %#v", out)
	}

	_, err = toolByName(t, p, "remote__fail").Run(context.Background(), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "壊れています") {
		t.Fatalf("err = %v", err)
	}
	if p.Instructions() == "" {
		t.Fatal("ツールがあるのに指示文が空です")
	}
}

func TestStdioServerTools(t *testing.T) {
	p := newPlugin(t, Config{Servers: map[string]ServerConfig{
		"local": {Command: os.Args[0], Env: map[string]string{stdioEnv: "1", "MCP_TEST_SUFFIX": "stdio"}, AllowedTools: []string{"echo"}},
	}})
	out, err := toolByName(t, p, "local__echo").Run(context.Background(), map[string]any{"text": "a"})
	if err != nil {
		t.Fatal(err)
	}
	if out["result"] != "echo: a stdio" {
		t.Fatalf("result = %#v", out)
	}
}

func TestDisabledAndUnreachableServers(t *testing.T) {
	disabled := false
	p := newPlugin(t, Config{Servers: map[string]ServerConfig{
		"off":  {URL: "http://127.0.0.1:1/mcp", Enabled: &disabled},
		"down": {URL: "http://127.0.0.1:1/mcp"},
	}})
	if len(p.Tools()) != 0 || p.Instructions() != "" {
		t.Fatalf("tools = %v", toolNames(p))
	}
}

func TestConfigValidation(t *testing.T) {
	cases := map[string]ServerConfig{
		"bad name!": {URL: "http://example.test"},
		"none":      {},
		"both":      {URL: "http://example.test", Command: "x"},
		"stdioAuth": {Command: "x", OAuth: &OAuthConfig{}},
		"httpArgs":  {URL: "http://example.test", Args: []string{"a"}},
	}
	for serverName, sc := range cases {
		_, err := New(plugin.Deps{DataDir: t.TempDir(), Config: func(_ string, v any) error {
			*(v.(*Config)) = Config{Servers: map[string]ServerConfig{serverName: sc}}
			return nil
		}})
		if err == nil {
			t.Errorf("%s: エラーになりません", serverName)
		}
	}
}

func TestToolParameters(t *testing.T) {
	params, err := toolParameters(json.RawMessage(`{"$schema":"x","type":"object"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := params["$schema"]; ok {
		t.Fatalf("$schema が残っています: %#v", params)
	}
	if params["properties"] == nil {
		t.Fatalf("properties がありません: %#v", params)
	}
}

func TestResultTextTruncates(t *testing.T) {
	long := strings.Repeat("あ", maxResultLength+10)
	text := resultText(&sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: long}}})
	if !strings.HasSuffix(text, "（長いため、ここで切り詰めました）") {
		t.Fatalf("切り詰められていません")
	}
	structured := resultText(&sdk.CallToolResult{StructuredContent: map[string]any{"a": 1}})
	if structured != `{"a":1}` {
		t.Fatalf("structured = %s", structured)
	}
}
