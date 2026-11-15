package claudecode

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/plugin"
)

func depsWithURL(url string) plugin.Deps {
	return plugin.Deps{Config: func(name string, v any) error {
		if name != "claudecode" {
			return nil
		}
		v.(*Config).URL = url
		return nil
	}}
}

// URL と共有トークンの両方があるときだけ、ツールと利用方針を出す。なければ通知の受け口は 503 を返す。
func TestPluginNeedsURLAndToken(t *testing.T) {
	for _, c := range []struct{ url, token string }{{"", ""}, {"http://bridge.test", ""}, {"", "token"}} {
		t.Setenv("AGENT_BRIDGE_TOKEN", c.token)
		p, err := New(depsWithURL(c.url))
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Tools()) != 0 || p.Instructions() != "" {
			t.Fatalf("設定が足りないのにツールか指示文が出ています: %+v", c)
		}
		mux := http.NewServeMux()
		p.RegisterHTTP(mux)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/agent-events", strings.NewReader("{}")))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d、期待値 = %d", rec.Code, http.StatusServiceUnavailable)
		}
	}

	t.Setenv("AGENT_BRIDGE_TOKEN", "token")
	p, err := New(depsWithURL("http://bridge.test"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range p.Tools() {
		names = append(names, tool.Name())
	}
	if len(names) != len(Operations) || names[0] != "ask_claude_code" {
		t.Fatalf("ツール = %v", names)
	}
	if !strings.Contains(p.Instructions(), "Claude Codeの利用方針") {
		t.Fatalf("指示文 = %q", p.Instructions())
	}
}

func TestPluginReportsConfigError(t *testing.T) {
	if _, err := New(plugin.Deps{Config: func(string, any) error { return http.ErrNotSupported }}); err == nil {
		t.Fatal("設定の読み込みの失敗がエラーになりません")
	}
}
