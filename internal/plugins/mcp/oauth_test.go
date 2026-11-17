package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tetetratra/smart-speaker/internal/plugin"
)

// fakeAuthServer は、MCP サーバー・保護リソースのメタデータ・認可サーバーを1つにまとめたテスト用のサーバー。
// MCP サーバーは access-2 だけを受け付けるので、認可で得た access-1 が更新されないとツールを取れない。
type fakeAuthServer struct {
	*httptest.Server
	mu         sync.Mutex
	registered []string
	tokenForms []url.Values
}

func newFakeAuthServer(t *testing.T) *fakeAuthServer {
	t.Helper()
	f := &fakeAuthServer{}
	mcpServer := newTestMCPServer()
	mcpHandler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return mcpServer }, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-2" {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+f.URL+`/custom-prm"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /custom-prm", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"resource": f.URL + "/mcp", "authorization_servers": []string{f.URL + "/as"}, "scopes_supported": []string{"read", "write"}})
	})
	mux.HandleFunc("GET /.well-known/oauth-authorization-server/as", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                 f.URL + "/as",
			"authorization_endpoint": f.URL + "/as/authorize",
			"token_endpoint":         f.URL + "/as/token",
			"registration_endpoint":  f.URL + "/as/register",
		})
	})
	mux.HandleFunc("POST /as/register", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			RedirectURIs []string `json:"redirect_uris"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.registered = append(f.registered, body.RedirectURIs...)
		f.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]any{"client_id": "dynamic-client"})
	})
	mux.HandleFunc("POST /as/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.tokenForms = append(f.tokenForms, r.PostForm)
		f.mu.Unlock()
		switch r.PostForm.Get("grant_type") {
		case "authorization_code":
			// すぐ期限切れになるトークンを返し、使うときに更新させる。
			writeJSON(w, map[string]any{"access_token": "access-1", "refresh_token": "refresh-1", "token_type": "Bearer", "expires_in": 1})
		case "refresh_token":
			writeJSON(w, map[string]any{"access_token": "access-2", "refresh_token": "refresh-2", "token_type": "Bearer", "expires_in": 3600})
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(func() { f.CloseClientConnections(); f.Close() })
	return f
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func newPluginIn(t *testing.T, dataDir string, cfg Config) *Plugin {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	p, err := New(plugin.Deps{AppContext: ctx, DataDir: dataDir, Config: func(_ string, v any) error {
		*(v.(*Config)) = cfg
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOAuthFlow(t *testing.T) {
	f := newFakeAuthServer(t)
	dataDir := t.TempDir()
	cfg := Config{
		RedirectURL: "http://localhost:8081/oauth/mcp/callback",
		Servers:     map[string]ServerConfig{"auth": {URL: f.URL + "/mcp", OAuth: &OAuthConfig{}, AllowedTools: []string{"echo"}}},
	}

	// 認可する前は、ツールを出さずに起動する。
	p := newPluginIn(t, dataDir, cfg)
	if len(p.Tools()) != 0 {
		t.Fatalf("認可前なのにツールがあります: %v", toolNames(p))
	}
	mux := http.NewServeMux()
	p.RegisterHTTP(mux)

	// 認可の開始: 動的クライアント登録をして、PKCE と resource つきで認可サーバーへリダイレクトする。
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/oauth/mcp/start?server=auth", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	q := location.Query()
	if location.Path != "/as/authorize" || q.Get("client_id") != "dynamic-client" || q.Get("code_challenge_method") != "S256" ||
		q.Get("code_challenge") == "" || q.Get("resource") != f.URL+"/mcp" || q.Get("scope") != "read write" || q.Get("redirect_uri") != cfg.RedirectURL {
		t.Fatalf("authorize URL = %s", location)
	}
	if strings.Join(f.registered, ",") != cfg.RedirectURL {
		t.Fatalf("registered = %v", f.registered)
	}

	// コールバック: 認可コードをトークンに換えて保存する。
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/oauth/mcp/callback?code=the-code&state="+url.QueryEscape(q.Get("state")), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("callback: %d %s", rec.Code, rec.Body.String())
	}
	exchange := f.tokenForms[0]
	if exchange.Get("code") != "the-code" || exchange.Get("code_verifier") == "" || exchange.Get("resource") != f.URL+"/mcp" {
		t.Fatalf("token request = %v", exchange)
	}

	// 同じ state は使い回せない。
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/oauth/mcp/callback?code=the-code&state="+url.QueryEscape(q.Get("state")), nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("state を使い回せてしまいます: %d", rec.Code)
	}

	// 再起動すると、保存したトークンを更新してからツールを取る。更新したトークンは保存し直す。
	p = newPluginIn(t, dataDir, cfg)
	out, err := toolByName(t, p, "auth__echo").Run(context.Background(), map[string]any{"text": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out["result"].(string), "echo: x") {
		t.Fatalf("result = %#v", out)
	}
	data, err := os.ReadFile(filepath.Join(dataDir, "mcp_oauth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "access-2") || !strings.Contains(string(data), "dynamic-client") {
		t.Fatalf("保存した内容 = %s", data)
	}
}

func TestOAuthStartUnknownServer(t *testing.T) {
	p := newPlugin(t, Config{})
	mux := http.NewServeMux()
	p.RegisterHTTP(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/oauth/mcp/start?server=nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d", rec.Code)
	}
}

func TestAuthRequiredWithoutToken(t *testing.T) {
	f := newFakeAuthServer(t)
	p := newPlugin(t, Config{Servers: map[string]ServerConfig{"auth": {URL: f.URL + "/mcp", OAuth: &OAuthConfig{}}}})
	_, err := p.oauth.token(context.Background(), "auth")
	if err == nil || !strings.Contains(err.Error(), "/oauth/mcp/start?server=auth") {
		t.Fatalf("err = %v", err)
	}
}
