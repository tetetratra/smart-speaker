package registry

import (
	diarystore "smart-speaker/internal/diary"
	calendarapi "smart-speaker/internal/googlecalendar"
	"smart-speaker/internal/tools"
	diarytool "smart-speaker/internal/tools/functions/diary"
	"smart-speaker/internal/tools/functions/googlecalendar"
	"smart-speaker/internal/tools/functions/switchbot"
	"smart-speaker/internal/tools/functions/timer"
	"smart-speaker/internal/tools/functions/volume"
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
	CalendarClient     *calendarapi.Client
	DiaryStore         *diarystore.Store
}

// New は利用可能なツールをまとめて登録します。
func New(cfg Config) *Registry {
	var entries []entry

	airconTool := switchbot.NewAircon(cfg.SwitchBotToken, cfg.SwitchBotSecret, cfg.SwitchBotDeviceMap)
	lightTool := switchbot.NewLight(cfg.SwitchBotToken, cfg.SwitchBotSecret, cfg.SwitchBotDeviceMap)
	blindTool := switchbot.NewBlindTilt(cfg.SwitchBotToken, cfg.SwitchBotSecret, cfg.SwitchBotDeviceMap)
	hub2Tool := switchbot.NewHub2(cfg.SwitchBotToken, cfg.SwitchBotSecret, cfg.SwitchBotDeviceMap)
	diaryTool := diarytool.New(cfg.DiaryStore)
	googleCalendarList := googlecalendar.NewList(cfg.CalendarClient)
	googleCalendarCreate := googlecalendar.NewCreate(cfg.CalendarClient)
	googleCalendarUpdate := googlecalendar.NewUpdate(cfg.CalendarClient)
	timerTool := timer.New()
	volumeTool := volume.New()
	toolEntries := []entry{
		{def: airconTool.Definition(), handler: airconTool},
		{def: lightTool.Definition(), handler: lightTool},
		{def: blindTool.Definition(), handler: blindTool},
		{def: hub2Tool.Definition(), handler: hub2Tool},
		{def: timerTool.Definition(), handler: timerTool},
		{def: volumeTool.Definition(), handler: volumeTool},
		{def: diaryTool.Definition(), handler: diaryTool},
		{def: googleCalendarList.Definition(), handler: googleCalendarList},
		{def: googleCalendarCreate.Definition(), handler: googleCalendarCreate},
		{def: googleCalendarUpdate.Definition(), handler: googleCalendarUpdate},
		{def: map[string]any{"type": "web_search"}},
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

// DefinitionsExcluding returns tool definitions excluding specified tool names.
func (r *Registry) DefinitionsExcluding(names ...string) []any {
	if r == nil {
		return nil
	}
	exclude := map[string]struct{}{}
	for _, name := range names {
		exclude[name] = struct{}{}
	}
	defs := make([]any, 0, len(r.entries))
	for _, e := range r.entries {
		if e.def == nil {
			continue
		}
		if name, ok := e.def["name"].(string); ok {
			if _, found := exclude[name]; found {
				continue
			}
		}
		defs = append(defs, e.def)
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
