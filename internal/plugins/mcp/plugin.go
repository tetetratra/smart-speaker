// Package mcp は、config.yaml に登録した MCP サーバーのツールを AI ツールとして出すプラグイン。
//
// Go サーバーが MCP クライアントになり、起動時に各サーバーへ接続してツールの一覧を取る。
// ツールは「<サーバー名>__<ツール名>」の function ツールとして会話モデルに渡し、呼ばれたら MCP の tools/call に中継する。
// 登録・変更・無効化は config.yaml の plugins.mcp に書き、再起動で反映する。
//
// 認可が要るリモートの MCP サーバーは、MCP 標準の OAuth（保護リソースのメタデータ・認可サーバーのメタデータ・
// 動的クライアント登録・PKCE）で認可する。/oauth/mcp/start?server=<名前> をブラウザで開いて認可し、再起動するとツールが出る。
package mcp

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tetetratra/smart-speaker/internal/plugin"
	"github.com/tetetratra/smart-speaker/internal/tools"
)

const (
	name = "mcp"
	// DefaultRedirectURL は、設定で指定しない場合の OAuth のコールバック URL。
	DefaultRedirectURL = "http://localhost:8081/oauth/mcp/callback"
	// connectTimeout は、起動時に1つのサーバーへ接続してツールの一覧を取るまでの待ち時間。
	connectTimeout = 30 * time.Second
	// oauthHTTPTimeout は、メタデータの取得・動的クライアント登録・トークンの取得と更新の HTTP の待ち時間。
	// トークンの更新はロックを持ったまま待つので、応答しない認可サーバーで起動やツール呼び出しが止まらないようにする。
	oauthHTTPTimeout = 30 * time.Second
)

// Config は config.yaml の plugins.mcp。
type Config struct {
	// RedirectURL は OAuth のコールバック URL。
	RedirectURL string `yaml:"redirect_url"`
	// TokenPath は OAuth のトークンの保存先。空ならデータの置き場所（Deps.DataDir）の mcp_oauth.json。
	TokenPath string `yaml:"token_path"`
	// Servers は MCP サーバー。キーはサーバー名で、ツール名の接頭辞になる。
	Servers map[string]ServerConfig `yaml:"servers"`
}

// ServerConfig は MCP サーバー1つの設定。URL（リモート）と Command（stdio）のどちらか一方を書く。
// URL・Headers・Command・Args・Env・OAuth の文字列の ${VAR} は環境変数で展開する。秘密の値は環境変数で渡す。
type ServerConfig struct {
	// Enabled が false なら接続しない。書かなければ有効。
	Enabled *bool `yaml:"enabled"`
	// URL は Streamable HTTP のリモート MCP サーバーの URL。
	URL string `yaml:"url"`
	// Headers はリモートの MCP サーバーへのリクエストに足すヘッダー（固定トークンなど）。
	Headers map[string]string `yaml:"headers"`
	// Command は stdio のローカル MCP サーバーの起動コマンド。
	Command string `yaml:"command"`
	// Args は Command の引数。
	Args []string `yaml:"args"`
	// Env は Command に足す環境変数。サーバー自身の環境変数も引き継ぐ。
	Env map[string]string `yaml:"env"`
	// AllowedTools は会話モデルに出すツール名（MCP サーバー上の名前）。空なら全部出す。
	AllowedTools []string `yaml:"allowed_tools"`
	// OAuth を書くと、リモートの MCP サーバーへ OAuth で認可したトークンを付けて接続する。
	OAuth *OAuthConfig `yaml:"oauth"`
}

// OAuthConfig は OAuth の設定。動的クライアント登録に対応したサーバーなら、何も書かなくてよい。
type OAuthConfig struct {
	// ClientID と ClientSecret は、事前に作った OAuth クライアント。空なら動的クライアント登録を使う。
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	// Scopes は要求するスコープ。空ならサーバーのメタデータの scopes_supported を全部要求する。
	Scopes []string `yaml:"scopes"`
}

var serverNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func (c ServerConfig) enabled() bool { return c.Enabled == nil || *c.Enabled }

// expanded は、文字列の ${VAR} を環境変数で展開した設定を返す。
func (c ServerConfig) expanded() ServerConfig {
	c.URL = os.ExpandEnv(c.URL)
	c.Command = os.ExpandEnv(c.Command)
	c.Headers = expandMap(c.Headers)
	c.Env = expandMap(c.Env)
	args := make([]string, len(c.Args))
	for i, arg := range c.Args {
		args[i] = os.ExpandEnv(arg)
	}
	c.Args = args
	if c.OAuth != nil {
		oauth := *c.OAuth
		oauth.ClientID = strings.TrimSpace(os.ExpandEnv(oauth.ClientID))
		oauth.ClientSecret = strings.TrimSpace(os.ExpandEnv(oauth.ClientSecret))
		c.OAuth = &oauth
	}
	return c
}

func expandMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = os.ExpandEnv(v)
	}
	return out
}

func (c ServerConfig) validate(serverName string) error {
	if !serverNamePattern.MatchString(serverName) {
		return fmt.Errorf("config: plugins.mcp.servers.%s: サーバー名に使えるのは英数字・_・- だけです", serverName)
	}
	if (c.URL == "") == (c.Command == "") {
		return fmt.Errorf("config: plugins.mcp.servers.%s: url と command のどちらか一方を書いてください", serverName)
	}
	if c.Command != "" && (len(c.Headers) > 0 || c.OAuth != nil) {
		return fmt.Errorf("config: plugins.mcp.servers.%s: headers と oauth は url のサーバーにだけ書けます", serverName)
	}
	if c.URL != "" && (len(c.Args) > 0 || len(c.Env) > 0) {
		return fmt.Errorf("config: plugins.mcp.servers.%s: args と env は command のサーバーにだけ書けます", serverName)
	}
	return nil
}

// Plugin は MCP のプラグイン。
type Plugin struct {
	tools []tools.Handler
	oauth *oauthManager
}

// New は、有効な MCP サーバーへ並行して接続し、ツールの一覧を取る。
// 接続や認可に失敗したサーバーはログに出してツールを出さない。ほかのサーバーと本体の起動は止めない。
func New(deps plugin.Deps) (*Plugin, error) {
	var cfg Config
	if err := deps.DecodeConfig(name, &cfg); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(cfg.Servers))
	for serverName, sc := range cfg.Servers {
		if err := sc.validate(serverName); err != nil {
			return nil, err
		}
		names = append(names, serverName)
	}
	sort.Strings(names)

	ctx := deps.AppContext
	if ctx == nil {
		ctx = context.Background()
	}
	tokenPath := strings.TrimSpace(cfg.TokenPath)
	if tokenPath == "" {
		tokenPath = filepath.Join(deps.DataDir, "mcp_oauth.json")
	}
	redirectURL := strings.TrimSpace(cfg.RedirectURL)
	if redirectURL == "" {
		redirectURL = DefaultRedirectURL
	}
	oauth := newOAuthManager(redirectURL, &tokenStore{path: tokenPath}, &http.Client{Timeout: oauthHTTPTimeout})

	servers := make([]*server, 0, len(names))
	for _, serverName := range names {
		sc := cfg.Servers[serverName]
		if !sc.enabled() {
			log.Printf("mcp: %s は無効なので接続しません", serverName)
			continue
		}
		sc = sc.expanded()
		if sc.OAuth != nil {
			oauth.add(serverName, sc.URL, *sc.OAuth)
		}
		servers = append(servers, newServer(ctx, serverName, sc, oauth))
	}

	toolsByServer := make([][]tools.Handler, len(servers))
	var wg sync.WaitGroup
	for i, s := range servers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			listCtx, cancel := context.WithTimeout(ctx, connectTimeout)
			defer cancel()
			handlers, err := s.listTools(listCtx)
			if err != nil {
				log.Printf("mcp: %s のツールを取れませんでした: %v", s.name, err)
				return
			}
			log.Printf("mcp: %s のツールを %d 個読み込みました", s.name, len(handlers))
			toolsByServer[i] = handlers
		}()
	}
	wg.Wait()

	p := &Plugin{oauth: oauth}
	for _, handlers := range toolsByServer {
		p.tools = append(p.tools, handlers...)
	}
	return p, nil
}

func (p *Plugin) Name() string { return name }

func (p *Plugin) Tools() []tools.Handler { return p.tools }

// RegisterHTTP は OAuth の認可の開始とコールバックを登録する。
func (p *Plugin) RegisterHTTP(mux *http.ServeMux) { p.oauth.registerHTTP(mux) }

// Instructions は、MCP のツールで外部の状態を変える前に確認するよう指示する。ツールがなければ空。
func (p *Plugin) Instructions() string {
	if len(p.tools) == 0 {
		return ""
	}
	return `## MCPツールの利用方針
- 名前が「<サーバー名>__」で始まるツールは、外部サービスにつながるMCPツールです。
- 作成・更新・削除・送信など外部の状態を変えるMCPツールは、実行する内容をユーザーに短く確認し、了承を得てから呼び出してください。参照だけのツールは確認せずに呼び出して構いません。`
}
