package googlecalendar

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/plugin"
)

func toolNames(p *Plugin) string {
	var names []string
	for _, t := range p.Tools() {
		names = append(names, t.Name())
	}
	return strings.Join(names, ",")
}

// OAuth クライアントの ID とシークレットがそろったときだけ、ツールを出す。
func TestPluginNeedsClientCredentials(t *testing.T) {
	deps := plugin.Deps{DataDir: t.TempDir()}
	for _, env := range [][2]string{{"", ""}, {"id", ""}, {"", "secret"}} {
		t.Setenv("GOOGLE_CLIENT_ID", env[0])
		t.Setenv("GOOGLE_CLIENT_SECRET", env[1])
		p, err := New(deps)
		if err != nil {
			t.Fatal(err)
		}
		if got := toolNames(p); got != "" {
			t.Fatalf("ID かシークレットがないのにツールが出ています: %s", got)
		}
	}
	t.Setenv("GOOGLE_CLIENT_ID", "id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "secret")
	p, err := New(deps)
	if err != nil {
		t.Fatal(err)
	}
	if got := toolNames(p); got != "google_calendar_list,google_calendar_create,google_calendar_update" {
		t.Fatalf("ツール = %s", got)
	}
}

// 認可の HTTP は、ツールの有無によらず登録する。
func TestPluginRegistersOAuthHTTP(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "")
	p, err := New(plugin.Deps{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	p.RegisterHTTP(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/oauth/google/status", nil))
	if rec.Code == http.StatusNotFound {
		t.Fatal("/oauth/google/status が登録されていません")
	}
}

// config.yaml の plugins.googlecalendar を読み、読み込みの失敗はエラーにする。
func TestPluginReadsConfig(t *testing.T) {
	var gotName string
	deps := plugin.Deps{DataDir: t.TempDir(), Config: func(name string, v any) error {
		gotName = name
		return nil
	}}
	if _, err := New(deps); err != nil || gotName != "googlecalendar" {
		t.Fatalf("name = %q, err = %v", gotName, err)
	}
	deps.Config = func(string, any) error { return http.ErrNotSupported }
	if _, err := New(deps); err == nil {
		t.Fatal("設定の読み込みの失敗がエラーになりません")
	}
}
