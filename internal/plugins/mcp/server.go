package mcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tetetratra/smart-speaker/internal/tools"
)

// callTimeout は、ツール呼び出し1回（接続の張り直しを含む）の待ち時間。応答しないサーバーで会話が止まらないようにする。
var callTimeout = 60 * time.Second

// server は MCP サーバー1つへの接続。接続は最初に使うときに張り、失敗したら次の呼び出しで張り直す。
type server struct {
	// appCtx はアプリが動いている間だけ有効な context。stdio のプロセスと接続の寿命に使う。
	appCtx context.Context
	name   string
	cfg    ServerConfig
	oauth  *oauthManager
	client *sdk.Client

	mu      sync.Mutex
	session *sdk.ClientSession
}

func newServer(appCtx context.Context, serverName string, cfg ServerConfig, oauth *oauthManager) *server {
	return &server{
		appCtx: appCtx,
		name:   serverName,
		cfg:    cfg,
		oauth:  oauth,
		client: sdk.NewClient(&sdk.Implementation{Name: "smart-speaker", Version: "1.0.0"}, nil),
	}
}

// listTools はツールの一覧を取り、allowed_tools で絞って AI ツールにする。
func (s *server) listTools(ctx context.Context) ([]tools.Handler, error) {
	session, err := s.currentSession(ctx)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	for _, toolName := range s.cfg.AllowedTools {
		allowed[toolName] = true
	}
	var handlers []tools.Handler
	seen := map[string]bool{}
	names := map[string]string{}
	for t, err := range session.Tools(ctx, nil) {
		if err != nil {
			s.reset(session)
			return nil, err
		}
		if len(allowed) > 0 && !allowed[t.Name] {
			continue
		}
		seen[t.Name] = true
		h, err := newTool(s, t)
		if err != nil {
			log.Printf("mcp: %s のツール %q を飛ばします: %v", s.name, t.Name, err)
			continue
		}
		// 使えない文字を _ に置き換えると、別のツールと同じ名前になることがある。
		if other, ok := names[h.name]; ok {
			log.Printf("mcp: %s のツール %q は %q と同じ名前 %s になるので飛ばします", s.name, t.Name, other, h.name)
			continue
		}
		names[h.name] = t.Name
		handlers = append(handlers, h)
	}
	for _, toolName := range s.cfg.AllowedTools {
		if !seen[toolName] {
			log.Printf("mcp: %s の allowed_tools にある %q はサーバーにありません", s.name, toolName)
		}
	}
	return handlers, nil
}

// callTool はツールを呼ぶ。通信やプロトコルの失敗では接続を捨て、次の呼び出しで張り直す。
// 書き込みのツールを二重に実行しないよう、失敗した呼び出しはやり直さない。
func (s *server) callTool(ctx context.Context, toolName string, args map[string]any) (*sdk.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	session, err := s.currentSession(ctx)
	if err != nil {
		return nil, err
	}
	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: toolName, Arguments: args})
	if err != nil {
		s.reset(session)
		return nil, err
	}
	return result, nil
}

func (s *server) currentSession(ctx context.Context) (*sdk.ClientSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil {
		return s.session, nil
	}
	transport, err := s.transport()
	if err != nil {
		return nil, err
	}
	session, err := s.client.Connect(ctx, transport, nil)
	if err != nil {
		var authErr *authRequiredError
		if errors.As(err, &authErr) {
			return nil, authErr
		}
		return nil, fmt.Errorf("接続できません: %w", err)
	}
	s.session = session
	return session, nil
}

// reset は、失敗した接続を捨てる。ほかの呼び出しが張り直したあとなら何もしない。
func (s *server) reset(session *sdk.ClientSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == session {
		// Close は実行中の呼び出しが終わるのを待つことがあるので、待たずに捨てる。
		go func() { _ = session.Close() }()
		s.session = nil
	}
}

func (s *server) transport() (sdk.Transport, error) {
	if s.cfg.Command != "" {
		// stdio のプロセスはアプリの終了とともに止める。
		cmd := exec.CommandContext(s.appCtx, s.cfg.Command, s.cfg.Args...)
		cmd.Env = os.Environ()
		for k, v := range s.cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		cmd.Stderr = os.Stderr
		return &sdk.CommandTransport{Command: cmd}, nil
	}
	var rt http.RoundTripper = http.DefaultTransport
	if s.cfg.OAuth != nil {
		rt = &oauthTransport{base: rt, oauth: s.oauth, server: s.name}
	}
	if len(s.cfg.Headers) > 0 {
		rt = &headerTransport{base: rt, headers: s.cfg.Headers}
	}
	return &sdk.StreamableClientTransport{Endpoint: s.cfg.URL, HTTPClient: &http.Client{Transport: rt}}, nil
}

// headerTransport は、設定のヘッダーをリクエストに足す。
type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	return t.base.RoundTrip(req)
}
