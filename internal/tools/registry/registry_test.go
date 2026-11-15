package registry

import (
	"context"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	"path/filepath"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/tools/functions/switchbot"
)

func TestRegistrySwitchBotTools(t *testing.T) {
	client := switchbot.NewSwitchbotClient("token", "secret", map[string]string{"hub2": "hub-device"})
	reg := New(Config{
		SwitchBotClient: client,
		SwitchBotScenes: []switchbot.Scene{{SceneID: "scene-1", SceneName: "換気扇をつける"}},
	})

	if _, ok := definitionByName(reg, "switchbot_execute_scene"); !ok {
		t.Fatal("switchbot_execute_scene should be registered")
	}
	for _, name := range []string{"hub2_get_temperature", "hub2_get_humidity", "hub2_get_light_level"} {
		if _, ok := definitionByName(reg, name); !ok {
			t.Fatalf("%s should be registered", name)
		}
	}
	if _, ok := definitionByName(reg, "hub2_get_environment"); ok {
		t.Fatal("hub2_get_environment should not be registered")
	}
	if _, ok := definitionByName(reg, "set_whiteboard"); !ok {
		t.Fatal("set_whiteboard should be registered")
	}
	if _, ok := reg.Handlers()["set_whiteboard"]; !ok {
		t.Fatal("set_whiteboard handler should be registered")
	}
	if _, ok := definitionByName(reg, "write_diary"); ok {
		t.Fatal("write_diary should not be registered")
	}
	if _, ok := reg.Handlers()["write_diary"]; ok {
		t.Fatal("write_diary handler should not be registered")
	}
	for _, name := range []string{"aircon_control", "light_control", "blind_control", "switchbot_control_device"} {
		if _, ok := definitionByName(reg, name); ok {
			t.Fatalf("%s should not be registered", name)
		}
	}
	assertRegistered(t, reg, "switchbot_execute_scene")
	for _, name := range []string{"hub2_get_temperature", "hub2_get_humidity", "hub2_get_light_level"} {
		assertRegistered(t, reg, name)
	}
	assertRegistered(t, reg, "set_whiteboard")
	assertRegistered(t, reg, "google_calendar_list")
	assertRegistered(t, reg, "google_calendar_create")
	assertRegistered(t, reg, "google_calendar_update")
}

func TestRegistryOmitsSceneToolWithoutScenes(t *testing.T) {
	client := switchbot.NewSwitchbotClient("token", "secret", map[string]string{"hub2": "hub-device"})
	reg := New(Config{SwitchBotClient: client})

	if _, ok := definitionByName(reg, "switchbot_execute_scene"); ok {
		t.Fatal("switchbot_execute_scene should not be registered")
	}
	for _, name := range []string{"hub2_get_temperature", "hub2_get_humidity", "hub2_get_light_level"} {
		if _, ok := definitionByName(reg, name); !ok {
			t.Fatalf("%s should be registered", name)
		}
	}
	if _, ok := definitionByName(reg, "hub2_get_environment"); ok {
		t.Fatal("hub2_get_environment should not be registered")
	}
}

func TestRegistryOmitsSwitchBotToolsWithoutCredentials(t *testing.T) {
	reg := New(Config{})

	for _, name := range []string{"switchbot_execute_scene", "hub2_get_temperature", "hub2_get_humidity", "hub2_get_light_level"} {
		if _, ok := definitionByName(reg, name); ok {
			t.Fatalf("%s should not be registered", name)
		}
		if _, ok := reg.Handlers()[name]; ok {
			t.Fatalf("%s handler should not be registered", name)
		}
	}
	if _, ok := definitionByName(reg, "set_whiteboard"); !ok {
		t.Fatal("set_whiteboard should be registered")
	}
	for _, name := range []string{"google_calendar_list", "google_calendar_create", "google_calendar_update"} {
		if _, ok := definitionByName(reg, name); !ok {
			t.Fatalf("%s should be registered", name)
		}
		if _, ok := reg.Handlers()[name]; !ok {
			t.Fatalf("%s handler should be registered", name)
		}
	}
}

func TestRegistryDefinitionsForRealtimeIncludesSetWhiteboard(t *testing.T) {
	reg := New(Config{})

	found := false
	for _, def := range reg.DefinitionsForRealtime() {
		if m, ok := def.(map[string]any); ok && m["name"] == "set_whiteboard" {
			found = true
		}
	}
	if !found {
		t.Fatal("DefinitionsForRealtime should include set_whiteboard")
	}
	if _, ok := reg.Handlers()["set_whiteboard"]; !ok {
		t.Fatal("set_whiteboard handler should remain registered")
	}
}

func TestRegistryOmitsSetVolumeTool(t *testing.T) {
	client := switchbot.NewSwitchbotClient("token", "secret", map[string]string{"hub2": "hub-device"})
	reg := New(Config{SwitchBotClient: client})

	if _, ok := definitionByName(reg, "set_volume"); ok {
		t.Fatal("set_volume should not be registered")
	}
	if _, ok := reg.Handlers()["set_volume"]; ok {
		t.Fatal("set_volume handler should not be registered")
	}
}

func TestRegistryOmitsScheduleTimerTool(t *testing.T) {
	client := switchbot.NewSwitchbotClient("token", "secret", map[string]string{"hub2": "hub-device"})
	reg := New(Config{SwitchBotClient: client})

	if _, ok := definitionByName(reg, "schedule_timer"); ok {
		t.Fatal("schedule_timer should not be registered")
	}
	if _, ok := reg.Handlers()["schedule_timer"]; ok {
		t.Fatal("schedule_timer handler should not be registered")
	}
}

// メモリの保存先があるときだけ、メモリのツールを登録する。
func TestRegistryMemoryToolsOnlyWithStore(t *testing.T) {
	store, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memories.json"))
	if err != nil {
		t.Fatal(err)
	}
	reg := New(Config{Memory: store})
	assertRegistered(t, reg, "list_memories")
	for _, name := range []string{"add_memory", "update_memory", "delete_memory"} {
		assertRegistered(t, reg, name)
	}
	if _, ok := New(Config{}).Handlers()["add_memory"]; ok {
		t.Fatal("保存先がないのにメモリのツールが登録されました")
	}
}

func assertRegistered(t *testing.T, reg *Registry, name string) {
	t.Helper()
	if _, ok := definitionByName(reg, name); !ok {
		t.Fatalf("%s should be registered", name)
	}
}

// definitionByName は登録済みの tool 定義（アプリ内のメタデータを含む）を名前で探す。
func definitionByName(r *Registry, name string) (map[string]any, bool) {
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

// プラグインのツールは、定義を持つものだけ登録する。
func TestRegistryPluginTools(t *testing.T) {
	reg := New(Config{PluginTools: []tools.Handler{definedTool{name: "plugin_tool"}, undefinedTool{}}})
	assertRegistered(t, reg, "plugin_tool")
	if _, ok := reg.Handlers()["undefined_tool"]; ok {
		t.Fatal("定義のないツールが登録されました")
	}
}

type undefinedTool struct{}

func (undefinedTool) Name() string { return "undefined_tool" }
func (undefinedTool) Run(context.Context, map[string]any) (map[string]any, error) {
	return nil, nil
}

type definedTool struct{ name string }

func (t definedTool) Name() string { return t.name }
func (definedTool) Run(context.Context, map[string]any) (map[string]any, error) {
	return nil, nil
}
func (t definedTool) Definition() map[string]any {
	return map[string]any{"type": "function", "name": t.name}
}
