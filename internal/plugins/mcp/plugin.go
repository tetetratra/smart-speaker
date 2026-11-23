// Package mcp は、config.yaml に登録した MCP サーバーのツールを AI ツールとして出すプラグイン。
//
// Go サーバーが MCP クライアントになり、起動時に各サーバーへ接続してツールの一覧を取る。
// ツールは「<サーバー名>__<ツール名>」の function ツールとして会話モデルに渡し、呼ばれたら MCP の tools/call に中継する。
// 登録・変更・無効化は config.yaml の plugins.mcp に書き、サーバーの再起動で反映する。
//
// 認可が要るリモートの MCP サーバーは、MCP 標準の OAuth（保護リソースのメタデータ・認可サーバーのメタデータ・
// 動的クライアント登録・PKCE）で認可する。画面の「設定」から認可すると、その場でツールを読み込み直し、
// 再起動しなくても会話から使えるようになる。ツールが変わったことは OnToolsChanged で本体に知らせる。
// 設定画面にはサーバーごとの状態を GET /api/mcp/servers で返す。
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
	// connectTimeout は、1つのサーバーへ接続してツールの一覧を取るまでの待ち時間。起動時と、認可したあとの読み込み直しで使う。
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
	// appCtx はアプリが動いている間だけ有効な context。認可したあとの読み込み直しに使う。
	appCtx context.Context
	oauth  *oauthManager
	// servers は、接続するサーバーをサーバー名で引く。無効なサーバーは入れない。作ったあとは変えない。
	servers map[string]*server

	mu sync.Mutex
	// toolsByServer は、サーバーごとに読み込んだツール。認可したあとに読み込み直すと差し替える。
	toolsByServer map[string][]tools.Handler
	// statuses はサーバーごとの状態。設定画面に出す。並びはサーバー名の順。
	statuses []serverStatus
	// listeners は、ツールが変わったときに呼ぶ関数。
	listeners []func()
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

	p := &Plugin{appCtx: ctx, oauth: oauth, servers: map[string]*server{}, toolsByServer: map[string][]tools.Handler{}}
	for _, serverName := range names {
		sc := cfg.Servers[serverName]
		p.statuses = append(p.statuses, serverStatus{Name: serverName, OAuth: sc.OAuth != nil})
		if !sc.enabled() {
			log.Printf("mcp: %s は無効なので接続しません", serverName)
			p.statuses[len(p.statuses)-1].State = stateDisabled
			continue
		}
		sc = sc.expanded()
		if sc.OAuth != nil {
			oauth.add(serverName, sc.URL, *sc.OAuth)
		}
		p.servers[serverName] = newServer(ctx, serverName, sc, oauth)
	}

	var wg sync.WaitGroup
	for serverName := range p.servers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.loadServer(ctx, serverName)
		}()
	}
	wg.Wait()
	oauth.onAuthorized = p.reloadServer
	return p, nil
}

// loadServer は、サーバーのツールの一覧を取り、そのサーバーのツールと状態を差し替える。
// 取れなかったときは、そのサーバーのツールを外し、状態を unauthorized か error にする。ツールが変わったら true を返す。
func (p *Plugin) loadServer(ctx context.Context, serverName string) bool {
	s := p.servers[serverName]
	listCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	handlers, err := s.listTools(listCtx)
	status := loadedStatus(len(handlers), err)
	if err != nil {
		log.Printf("mcp: %s のツールを取れませんでした: %v", serverName, err)
	} else {
		log.Printf("mcp: %s のツールを %d 個読み込みました", serverName, len(handlers))
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	changed := len(p.toolsByServer[serverName]) > 0 || len(handlers) > 0
	if len(handlers) > 0 {
		p.toolsByServer[serverName] = handlers
	} else {
		delete(p.toolsByServer, serverName)
	}
	for i := range p.statuses {
		if p.statuses[i].Name == serverName {
			p.statuses[i].State, p.statuses[i].ToolCount, p.statuses[i].Message = status.State, status.ToolCount, status.Message
		}
	}
	return changed
}

// reloadServer は、認可したサーバーのツールを読み込み直し、ツールが変わったら OnToolsChanged の関数を呼ぶ。
// 認可のコールバックから呼び、読み込み終えるまで（最大 connectTimeout）待つ。
func (p *Plugin) reloadServer(serverName string) {
	if _, ok := p.servers[serverName]; !ok {
		return
	}
	if !p.loadServer(p.appCtx, serverName) {
		return
	}
	p.mu.Lock()
	listeners := append([]func(){}, p.listeners...)
	p.mu.Unlock()
	// 呼ばれた側が Tools() や Instructions() を呼べるよう、ロックを放してから呼ぶ。
	for _, f := range listeners {
		f()
	}
}

// OnToolsChanged は、認可したあとにツールを読み込み直して、ツールが変わったときに f を呼ぶよう登録する。
func (p *Plugin) OnToolsChanged(f func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.listeners = append(p.listeners, f)
}

func (p *Plugin) Name() string { return name }

var _ plugin.ToolChangeNotifier = (*Plugin)(nil)

// Tools は、今読み込んでいるツールをサーバー名の順に並べて返す。返す slice は呼び出し側で変えてよい。
func (p *Plugin) Tools() []tools.Handler {
	p.mu.Lock()
	defer p.mu.Unlock()
	var handlers []tools.Handler
	for _, st := range p.statuses {
		handlers = append(handlers, p.toolsByServer[st.Name]...)
	}
	return handlers
}

// RegisterHTTP は OAuth の認可の開始とコールバックを登録する。
func (p *Plugin) RegisterHTTP(mux *http.ServeMux) {
	p.oauth.registerHTTP(mux)
	mux.HandleFunc("GET /api/mcp/servers", p.handleServers)
}

// Instructions は、MCP のツールで外部の状態を変える前に確認するよう指示する。今ツールがなければ空。
// 起動したあとに認可してツールが増えることがあるので、呼ぶたびに今のツールで決める。
func (p *Plugin) Instructions() string {
	p.mu.Lock()
	empty := len(p.toolsByServer) == 0
	p.mu.Unlock()
	if empty {
		return ""
	}
	return `## MCPツールの利用方針
- 名前が「<サーバー名>__」で始まるツールは、外部サービスにつながるMCPツールです。
- 作成・更新・削除・送信など外部の状態を変えるMCPツールは、実行する内容をユーザーに短く確認し、了承を得てから呼び出してください。参照だけのツールは確認せずに呼び出して構いません。`
}
