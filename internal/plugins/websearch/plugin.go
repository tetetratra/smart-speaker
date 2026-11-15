// Package websearch は、Web 検索のプラグイン。
//
// AI ツール web_search は検索を待たずに受け付けを返し、OpenAI Responses API の hosted web_search で調べた結果を、
// あとから会話へ届ける（Deps.Deliver）。OpenAI のキーと Responses のモデルは本体と共用する。
package websearch

import (
	"strings"

	"github.com/tetetratra/smart-speaker/internal/plugin"
	"github.com/tetetratra/smart-speaker/internal/tools"
)

// Plugin は Web 検索のプラグイン。OpenAI のキーかモデルがなければ、ツールを出さない。
type Plugin struct {
	tool *Tool
}

// New は Web 検索のプラグインを作る。
func New(deps plugin.Deps) *Plugin {
	if strings.TrimSpace(deps.OpenAI.APIKey) == "" || strings.TrimSpace(deps.OpenAI.ResponsesModel) == "" {
		return &Plugin{}
	}
	return &Plugin{tool: NewTool(Config{
		AppContext: deps.AppContext,
		APIKey:     deps.OpenAI.APIKey,
		Model:      deps.OpenAI.ResponsesModel,
		Deliver:    deps.Deliver,
	})}
}

func (p *Plugin) Name() string { return "websearch" }

func (p *Plugin) Tools() []tools.Handler {
	if p.tool == nil {
		return nil
	}
	return []tools.Handler{p.tool}
}

// Instructions は、ツールがあるときだけ使い方を伝える。
func (p *Plugin) Instructions() string {
	if p.tool == nil {
		return ""
	}
	return "## Web検索\n- 最新情報や外部情報はweb_searchで確認してください。結果の日付や地域が違う場合、または結果同士が矛盾する場合は、条件を指定して一度だけ調べ直し、それでも確認できなければ短くそう伝えてください。"
}
