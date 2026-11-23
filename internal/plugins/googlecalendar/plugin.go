// Package googlecalendar は、Google カレンダーのプラグイン。
//
// 予定の一覧・作成・更新の AI ツールと、認可のための OAuth の HTTP（/oauth/google/*）を持つ。
// API のクライアントは calendarapi、OAuth とトークンの保存は googleoauth にある。
//
// OAuth クライアントの ID とシークレットは環境変数 GOOGLE_CLIENT_ID・GOOGLE_CLIENT_SECRET で渡す。
// コールバック URL・スコープ・トークンの保存先は config.yaml の plugins.googlecalendar に書く。
package googlecalendar

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tetetratra/smart-speaker/internal/plugin"
	"github.com/tetetratra/smart-speaker/internal/plugins/googlecalendar/calendarapi"
	"github.com/tetetratra/smart-speaker/internal/plugins/googlecalendar/googleoauth"
	"github.com/tetetratra/smart-speaker/internal/tools"
)

const name = "googlecalendar"

// Config は config.yaml の plugins.googlecalendar。
type Config struct {
	// RedirectURL は OAuth のコールバック URL。
	RedirectURL string `yaml:"redirect_url"`
	// Scope は OAuth のスコープ。
	Scope string `yaml:"scope"`
	// TokenPath はトークンの保存先。空ならデータの置き場所（Deps.DataDir）の google_oauth_token.json。
	TokenPath string `yaml:"token_path"`
}

// Plugin は Google カレンダーのプラグイン。OAuth クライアントの ID かシークレットがなければ、ツールを出さない。
type Plugin struct {
	client *calendarapi.Client
}

// New は Google カレンダーのプラグインを作る。OAuth の設定は googleoauth のパッケージ全体で共有する。
func New(deps plugin.Deps) (*Plugin, error) {
	var cfg Config
	if err := deps.DecodeConfig(name, &cfg); err != nil {
		return nil, err
	}
	oauthCfg := googleoauth.Config{
		ClientID:     strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")),
		ClientSecret: strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET")),
		RedirectURL:  stringOr(cfg.RedirectURL, googleoauth.DefaultRedirectURL),
		Scopes:       []string{stringOr(cfg.Scope, googleoauth.DefaultScope)},
		TokenPath:    stringOr(cfg.TokenPath, filepath.Join(deps.DataDir, "google_oauth_token.json")),
	}
	googleoauth.Configure(oauthCfg)
	if oauthCfg.ClientID == "" || oauthCfg.ClientSecret == "" {
		log.Println("googlecalendar: GOOGLE_CLIENT_ID or GOOGLE_CLIENT_SECRET not set; google calendar tools disabled")
		return &Plugin{}, nil
	}
	if _, err := googleoauth.LoadToken(); err != nil {
		log.Println("google oauth token not found. open /oauth/google/start to authenticate")
	}
	return &Plugin{client: calendarapi.NewClient(calendarapi.Config{})}, nil
}

func stringOr(value, fallback string) string {
	if v := strings.TrimSpace(value); v != "" {
		return v
	}
	return fallback
}

func (p *Plugin) Name() string { return name }

func (p *Plugin) Tools() []tools.Handler {
	if p.client == nil {
		return nil
	}
	return []tools.Handler{NewList(p.client), NewCreate(p.client), NewUpdate(p.client)}
}

// RegisterHTTP は OAuth の HTTP を登録する。ID とシークレットがなければ、認可の開始は理由つきで失敗する。
func (p *Plugin) RegisterHTTP(mux *http.ServeMux) {
	googleoauth.RegisterHTTPHandlers(mux)
}
