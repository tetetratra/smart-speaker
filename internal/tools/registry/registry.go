package registry

import (
	"strings"

	calendarapi "smart-speaker/internal/googlecalendar"
	"smart-speaker/internal/tools"
	"smart-speaker/internal/tools/functions/googlecalendar"
	"smart-speaker/internal/tools/functions/switchbot"
	"smart-speaker/internal/tools/functions/whiteboard"
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
	SwitchBotToken     string
	SwitchBotSecret    string
	SwitchBotDeviceMap string
	SwitchBotClient    *switchbot.Client
	SwitchBotScenes    []switchbot.Scene
	CalendarClient     *calendarapi.Client
}

// New は利用可能なツールをまとめて登録します。
func New(cfg Config) *Registry {
	var entries []entry

	switchBotClient := cfg.SwitchBotClient
	if switchBotClient == nil && strings.TrimSpace(cfg.SwitchBotToken) != "" && strings.TrimSpace(cfg.SwitchBotSecret) != "" {
		switchBotClient = switchbot.NewSwitchbotClient(cfg.SwitchBotToken, cfg.SwitchBotSecret, cfg.SwitchBotDeviceMap)
	}
	hub2Tool := switchbot.NewHub2WithClient(switchBotClient)
	sceneTool := switchbot.NewScene(switchBotClient, cfg.SwitchBotScenes)
	calendarClient := cfg.CalendarClient
	if calendarClient == nil {
		calendarClient = calendarapi.NewClient(calendarapi.Config{})
	}
	googleCalendarList := googlecalendar.NewList(calendarClient)
	googleCalendarCreate := googlecalendar.NewCreate(calendarClient)
	googleCalendarUpdate := googlecalendar.NewUpdate(calendarClient)
	whiteboardTool := whiteboard.New()
	toolEntries := []entry{
		{def: whiteboardTool.Definition(), handler: whiteboardTool},
		{def: googleCalendarList.Definition(), handler: googleCalendarList},
		{def: googleCalendarCreate.Definition(), handler: googleCalendarCreate},
		{def: googleCalendarUpdate.Definition(), handler: googleCalendarUpdate},
	}
	if hub2Tool != nil {
		toolEntries = append([]entry{{def: hub2Tool.Definition(), handler: hub2Tool}}, toolEntries...)
	}
	if sceneTool != nil {
		toolEntries = append([]entry{{def: sceneTool.Definition(), handler: sceneTool}}, toolEntries...)
	}
	for _, e := range toolEntries {
		entries = append(entries, e)
	}

	return &Registry{entries: entries}
}

// Definitions はResponses API向けのtools定義を返します。
func (r *Registry) Definitions() []any {
	if r == nil {
		return nil
	}
	defs := make([]any, 0, len(r.entries))
	for _, e := range r.entries {
		if e.def != nil {
			defs = append(defs, e.def)
		}
	}
	return defs
}

// DefinitionByName returns a tool definition by name.
func (r *Registry) DefinitionByName(name string) (map[string]any, bool) {
	if r == nil {
		return nil, false
	}
	for _, e := range r.entries {
		if e.def == nil {
			continue
		}
		if defName, ok := e.def["name"].(string); ok && defName == name {
			return e.def, true
		}
	}
	return nil, false
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
