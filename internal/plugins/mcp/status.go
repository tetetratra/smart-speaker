package mcp

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
)

// サーバーの状態。設定画面に出す。
const (
	// stateReady は、ツールを読み込めて使える状態。
	stateReady = "ready"
	// stateUnauthorized は、OAuth の認可がまだか、切れていてツールを読み込めない状態。
	stateUnauthorized = "unauthorized"
	// stateRestartRequired は、起動したあとに認可した状態。ツールは再起動すると読み込む。
	stateRestartRequired = "restart_required"
	// stateError は、接続できないなど、認可のほかの理由でツールを読み込めない状態。
	stateError = "error"
	// stateDisabled は、設定で無効にしていて接続しない状態。
	stateDisabled = "disabled"
)

// serverStatus は、設定画面に出すサーバー1つの状態。
type serverStatus struct {
	Name string `json:"name"`
	// OAuth は、OAuth で認可するサーバーかどうか。
	OAuth bool   `json:"oauth"`
	State string `json:"state"`
	// ToolCount は読み込んだツールの数。
	ToolCount int `json:"tool_count"`
	// Message は、接続できない理由など、状態の補足。
	Message string `json:"message,omitempty"`
	// AuthURL は、認可を始める URL。認可が要るときだけ入れる。
	AuthURL string `json:"auth_url,omitempty"`
}

// startupStatus は、起動時にツールを読み込んだ結果から状態を決める。
func startupStatus(toolCount int, err error) serverStatus {
	var authErr *authRequiredError
	switch {
	case err == nil:
		return serverStatus{State: stateReady, ToolCount: toolCount}
	case errors.As(err, &authErr):
		return serverStatus{State: stateUnauthorized}
	default:
		return serverStatus{State: stateError, Message: err.Error()}
	}
}

// markAuthorized は、起動したあとに認可したサーバーを「再起動すると使える」にする。
func (p *Plugin) markAuthorized(serverName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.statuses {
		if p.statuses[i].Name == serverName && p.statuses[i].State != stateReady {
			p.statuses[i].State = stateRestartRequired
			p.statuses[i].Message = ""
		}
	}
}

// handleServers は、サーバーの状態の一覧を返す。認可が要るサーバーには、認可を始める URL を添える。
func (p *Plugin) handleServers(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	servers := make([]serverStatus, len(p.statuses))
	copy(servers, p.statuses)
	p.mu.Unlock()
	for i := range servers {
		if servers[i].OAuth && servers[i].State == stateUnauthorized {
			servers[i].AuthURL = "/oauth/mcp/start?server=" + url.QueryEscape(servers[i].Name)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"servers": servers}); err != nil {
		log.Printf("mcp: サーバーの状態を返せませんでした: %v", err)
	}
}
