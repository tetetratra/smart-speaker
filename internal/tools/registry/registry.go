package registry

import (
	"github.com/tetetratra/smart-speaker/internal/agentbridge"
	"github.com/tetetratra/smart-speaker/internal/automations"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	automationstool "github.com/tetetratra/smart-speaker/internal/tools/functions/automations"
	claudecodetool "github.com/tetetratra/smart-speaker/internal/tools/functions/claudecode"
	memorytool "github.com/tetetratra/smart-speaker/internal/tools/functions/memory"
	waketool "github.com/tetetratra/smart-speaker/internal/tools/functions/wakemode"
	"strings"

	calendarapi "github.com/tetetratra/smart-speaker/internal/googlecalendar"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/tools/functions/googlecalendar"
	"github.com/tetetratra/smart-speaker/internal/tools/functions/switchbot"
	"github.com/tetetratra/smart-speaker/internal/tools/functions/websearch"
	"github.com/tetetratra/smart-speaker/internal/tools/functions/whiteboard"
)

// Registry はツール定義とハンドラをまとめて管理します。
type Registry struct {
	entries []entry
}

type entry struct {
	def     map[string]any
	handler tools.Handler
}

// Config はツールの生成に必要な設定を持ちます。
type Config struct {
	WakeMode           *wakemode.Store
	Automations        *automations.Service
	Memory             *memorystate.Store
	ClaudeCode         *agentbridge.Service
	SwitchBotToken     string
	SwitchBotSecret    string
	SwitchBotDeviceMap map[string]string
	SwitchBotClient    *switchbot.Client
	SwitchBotScenes    []switchbot.Scene
	CalendarClient     *calendarapi.Client
	OpenAIAPIKey       string
	OpenAIModel        string
	WebSearchClient    websearch.SearchClient
	// WebSearchDeliver があれば、web_search は結果を後から会話へ届ける。
	WebSearchDeliver websearch.Deliver
}

// New は利用可能なツールをまとめて登録します。
func New(cfg Config) *Registry {
	var entries []entry

	switchBotClient := cfg.SwitchBotClient
	if switchBotClient == nil && strings.TrimSpace(cfg.SwitchBotToken) != "" && strings.TrimSpace(cfg.SwitchBotSecret) != "" {
		switchBotClient = switchbot.NewSwitchbotClient(cfg.SwitchBotToken, cfg.SwitchBotSecret, cfg.SwitchBotDeviceMap)
	}
	hub2Tools := switchbot.NewHub2ToolsWithClient(switchBotClient)
	sceneTool := switchbot.NewScene(switchBotClient, cfg.SwitchBotScenes)
	calendarClient := cfg.CalendarClient
	if calendarClient == nil {
		calendarClient = calendarapi.NewClient(calendarapi.Config{})
	}
	googleCalendarList := googlecalendar.NewList(calendarClient)
	googleCalendarCreate := googlecalendar.NewCreate(calendarClient)
	googleCalendarUpdate := googlecalendar.NewUpdate(calendarClient)
	whiteboardTool := whiteboard.New()
	webSearchTool := newWebSearchTool(cfg)
	toolEntries := []entry{
		{def: whiteboardTool.Definition(), handler: whiteboardTool},
		{def: googleCalendarList.Definition(), handler: googleCalendarList},
		{def: googleCalendarCreate.Definition(), handler: googleCalendarCreate},
		{def: googleCalendarUpdate.Definition(), handler: googleCalendarUpdate},
	}
	if cfg.WakeMode != nil {
		for _, readOnly := range []bool{true, false} {
			t := &waketool.Tool{Store: cfg.WakeMode, ReadOnly: readOnly}
			toolEntries = append(toolEntries, entry{def: t.Definition(), handler: t})
		}
	}
	if cfg.Automations != nil {
		for _, operation := range []string{"register_job", "list_jobs", "update_job", "delete_job"} {
			t := &automationstool.Tool{Service: cfg.Automations, Operation: operation}
			toolEntries = append(toolEntries, entry{def: t.Definition(), handler: t})
		}
	}
	if cfg.Memory != nil {
		for _, operation := range memorytool.Operations {
			t := &memorytool.Tool{Store: cfg.Memory, Operation: operation}
			toolEntries = append(toolEntries, entry{def: t.Definition(), handler: t})
		}
	}
	if cfg.ClaudeCode != nil {
		for _, operation := range claudecodetool.Operations {
			t := &claudecodetool.Tool{Service: cfg.ClaudeCode, Operation: operation}
			toolEntries = append(toolEntries, entry{def: t.Definition(), handler: t})
		}
	}
	if webSearchTool != nil {
		toolEntries = append(toolEntries, entry{def: webSearchTool.Definition(), handler: webSearchTool})
	}
	if len(hub2Tools) > 0 {
		hub2Entries := make([]entry, 0, len(hub2Tools))
		for _, hub2Tool := range hub2Tools {
			hub2Entries = append(hub2Entries, entry{def: hub2Tool.Definition(), handler: hub2Tool})
		}
		toolEntries = append(hub2Entries, toolEntries...)
	}
	if sceneTool != nil {
		toolEntries = append([]entry{{def: sceneTool.Definition(), handler: sceneTool}}, toolEntries...)
	}
	for _, e := range toolEntries {
		entries = append(entries, e)
	}

	return &Registry{entries: entries}
}

func newWebSearchTool(cfg Config) *websearch.Tool {
	if cfg.WebSearchClient != nil {
		return websearch.New(websearch.Config{Client: cfg.WebSearchClient, Deliver: cfg.WebSearchDeliver})
	}
	if strings.TrimSpace(cfg.OpenAIAPIKey) == "" || strings.TrimSpace(cfg.OpenAIModel) == "" {
		return nil
	}
	return websearch.New(websearch.Config{
		APIKey:  cfg.OpenAIAPIKey,
		Model:   cfg.OpenAIModel,
		Deliver: cfg.WebSearchDeliver,
	})
}

// Handlers はtoolcaller向けのname->handlerマップを返します。
func (r *Registry) Handlers() map[string]tools.Handler {
	handlers := map[string]tools.Handler{}
	if r == nil {
		return handlers
	}
	for _, e := range r.entries {
		if e.handler == nil {
			continue
		}
		handlers[e.handler.Name()] = e.handler
	}
	return handlers
}

// DefinitionsForRealtime は全ツールをfunction calling用に返す。
func (r *Registry) DefinitionsForRealtime() []any {
	defs := make([]any, 0, len(r.entries))
	for _, e := range r.entries {
		if e.def == nil {
			continue
		}
		def := map[string]any{}
		for _, key := range []string{"type", "name", "description", "parameters"} {
			if value, ok := e.def[key]; ok {
				def[key] = value
			}
		}
		defs = append(defs, def)
	}
	return defs
}
