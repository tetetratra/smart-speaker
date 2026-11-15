// Package claudecode は、手元の Mac で動く Claude Code を会話から使うプラグイン。
//
// 依頼・承認の回答・取り消し・一覧・読み出しの AI ツールと、Mac の agent-bridge（services/agent-bridge）から
// 承認依頼・完了・失敗の通知を受け取る HTTP（/api/agent-events）を持つ。通知は会話へ届ける（Deps.Deliver）。
// agent-bridge とのやりとりは agentbridge にある。
//
// 共有トークンは環境変数 AGENT_BRIDGE_TOKEN で渡す。agent-bridge の URL は config.yaml の plugins.claudecode.url に書く。
package claudecode

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/tetetratra/smart-speaker/internal/plugin"
	"github.com/tetetratra/smart-speaker/internal/plugins/claudecode/agentbridge"
	"github.com/tetetratra/smart-speaker/internal/tools"
)

const name = "claudecode"

// Config は config.yaml の plugins.claudecode。
type Config struct {
	// URL は手元の Mac で動く agent-bridge の URL。
	URL string `yaml:"url"`
}

// Plugin は Claude Code 連携のプラグイン。URL か共有トークンがなければ、ツールと指示文を出さない。
type Plugin struct {
	service *agentbridge.Service
	token   string
}

// New は Claude Code 連携のプラグインを作る。
func New(deps plugin.Deps) (*Plugin, error) {
	var cfg Config
	if err := deps.DecodeConfig(name, &cfg); err != nil {
		return nil, err
	}
	url := strings.TrimSpace(cfg.URL)
	token := strings.TrimSpace(os.Getenv("AGENT_BRIDGE_TOKEN"))
	if url == "" || token == "" {
		log.Println("agent-bridge: plugins.claudecode.url in config.yaml or AGENT_BRIDGE_TOKEN not set; claude code tools disabled")
		return &Plugin{}, nil
	}
	return &Plugin{service: agentbridge.NewService(agentbridge.NewClient(url, token), deps.Deliver), token: token}, nil
}

func (p *Plugin) Name() string { return name }

func (p *Plugin) Tools() []tools.Handler {
	if p.service == nil {
		return nil
	}
	handlers := make([]tools.Handler, 0, len(Operations))
	for _, operation := range Operations {
		handlers = append(handlers, &Tool{Service: p.service, Operation: operation})
	}
	return handlers
}

// RegisterHTTP は通知の受け口を登録する。無効なときは 503 を返す。
func (p *Plugin) RegisterHTTP(mux *http.ServeMux) {
	registerAgentEventsAPI(mux, p.service, p.token)
}

func (p *Plugin) Instructions() string {
	if p.service == nil {
		return ""
	}
	return instructions
}
