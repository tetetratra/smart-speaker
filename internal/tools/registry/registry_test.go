package registry

import (
	"testing"

	"github.com/tetetratra/smart-speaker/internal/tools/functions/switchbot"
)

func TestRegistrySwitchBotTools(t *testing.T) {
	client := switchbot.NewSwitchbotClient("token", "secret", `{"hub2":"hub-device"}`)
	reg := New(Config{
		SwitchBotClient: client,
		SwitchBotScenes: []switchbot.Scene{{SceneID: "scene-1", SceneName: "換気扇をつける"}},
	})

	if _, ok := reg.DefinitionByName("switchbot_execute_scene"); !ok {
		t.Fatal("switchbot_execute_scene should be registered")
	}
	for _, name := range []string{"hub2_get_temperature", "hub2_get_humidity", "hub2_get_light_level"} {
		if _, ok := reg.DefinitionByName(name); !ok {
			t.Fatalf("%s should be registered", name)
		}
	}
	if _, ok := reg.DefinitionByName("hub2_get_environment"); ok {
		t.Fatal("hub2_get_environment should not be registered")
	}
	if _, ok := reg.DefinitionByName("set_whiteboard"); !ok {
		t.Fatal("set_whiteboard should be registered")
	}
	if _, ok := reg.DefinitionByName("timer"); !ok {
		t.Fatal("timer should be registered")
	}
	if _, ok := reg.DefinitionByName("cancel_timer"); !ok {
		t.Fatal("cancel_timer should be registered")
	}
	if _, ok := reg.Handlers()["set_whiteboard"]; !ok {
		t.Fatal("set_whiteboard handler should be registered")
	}
	if _, ok := reg.Handlers()["timer"]; !ok {
		t.Fatal("timer handler should be registered")
	}
	if _, ok := reg.Handlers()["cancel_timer"]; !ok {
		t.Fatal("cancel_timer handler should be registered")
	}
	if _, ok := reg.DefinitionByName("write_diary"); ok {
		t.Fatal("write_diary should not be registered")
	}
	if _, ok := reg.Handlers()["write_diary"]; ok {
		t.Fatal("write_diary handler should not be registered")
	}
	for _, name := range []string{"aircon_control", "light_control", "blind_control", "switchbot_control_device"} {
		if _, ok := reg.DefinitionByName(name); ok {
			t.Fatalf("%s should not be registered", name)
		}
	}
	assertToolMode(t, reg, "switchbot_execute_scene", "write")
	for _, name := range []string{"hub2_get_temperature", "hub2_get_humidity", "hub2_get_light_level"} {
		assertToolMode(t, reg, name, "read")
	}
	assertToolMode(t, reg, "set_whiteboard", "write")
	assertToolMode(t, reg, "timer", "write")
	assertToolMode(t, reg, "cancel_timer", "write")
	assertToolMode(t, reg, "google_calendar_list", "read")
	assertToolMode(t, reg, "google_calendar_create", "write")
	assertToolMode(t, reg, "google_calendar_update", "write")
}

func TestRegistryOmitsSceneToolWithoutScenes(t *testing.T) {
	client := switchbot.NewSwitchbotClient("token", "secret", `{"hub2":"hub-device"}`)
	reg := New(Config{SwitchBotClient: client})

	if _, ok := reg.DefinitionByName("switchbot_execute_scene"); ok {
		t.Fatal("switchbot_execute_scene should not be registered")
	}
	for _, name := range []string{"hub2_get_temperature", "hub2_get_humidity", "hub2_get_light_level"} {
		if _, ok := reg.DefinitionByName(name); !ok {
			t.Fatalf("%s should be registered", name)
		}
	}
	if _, ok := reg.DefinitionByName("hub2_get_environment"); ok {
		t.Fatal("hub2_get_environment should not be registered")
	}
}

func TestRegistryOmitsSwitchBotToolsWithoutCredentials(t *testing.T) {
	reg := New(Config{})

	for _, name := range []string{"switchbot_execute_scene", "hub2_get_temperature", "hub2_get_humidity", "hub2_get_light_level"} {
		if _, ok := reg.DefinitionByName(name); ok {
			t.Fatalf("%s should not be registered", name)
		}
		if _, ok := reg.Handlers()[name]; ok {
			t.Fatalf("%s handler should not be registered", name)
		}
	}
	if _, ok := reg.DefinitionByName("set_whiteboard"); !ok {
		t.Fatal("set_whiteboard should be registered")
	}
	if _, ok := reg.DefinitionByName("timer"); !ok {
		t.Fatal("timer should be registered")
	}
	if _, ok := reg.DefinitionByName("cancel_timer"); !ok {
		t.Fatal("cancel_timer should be registered")
	}
	for _, name := range []string{"google_calendar_list", "google_calendar_create", "google_calendar_update"} {
		if _, ok := reg.DefinitionByName(name); !ok {
			t.Fatalf("%s should be registered", name)
		}
		if _, ok := reg.Handlers()[name]; !ok {
			t.Fatalf("%s handler should be registered", name)
		}
	}
}

func TestRegistryRegistersWebSearchToolWithOpenAIConfig(t *testing.T) {
	reg := New(Config{OpenAIAPIKey: "key", OpenAIModel: "model"})

	def, ok := reg.DefinitionByName("web_search")
	if !ok {
		t.Fatal("web_search should be registered")
	}
	params, ok := def["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters = %#v", def["parameters"])
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", params["properties"])
	}
	if _, ok := props["query"]; !ok || len(props) != 1 {
		t.Fatalf("properties = %#v, want query only", props)
	}
	if _, ok := reg.Handlers()["web_search"]; !ok {
		t.Fatal("web_search handler should be registered")
	}
	assertToolMode(t, reg, "web_search", "read")
}

func TestRegistryOmitsWebSearchToolWithoutOpenAIConfig(t *testing.T) {
	reg := New(Config{})

	if _, ok := reg.DefinitionByName("web_search"); ok {
		t.Fatal("web_search should not be registered")
	}
	if _, ok := reg.Handlers()["web_search"]; ok {
		t.Fatal("web_search handler should not be registered")
	}
}

func TestRegistryDefinitionsForLLMOmitsSetWhiteboard(t *testing.T) {
	reg := New(Config{})

	for _, def := range reg.DefinitionsForLLM() {
		m, ok := def.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := m["name"].(string); ok && name == "set_whiteboard" {
			t.Fatal("DefinitionsForLLM should not include set_whiteboard")
		}
	}
	if _, ok := reg.DefinitionByName("set_whiteboard"); !ok {
		t.Fatal("set_whiteboard should remain in Definitions")
	}
	if _, ok := reg.Handlers()["set_whiteboard"]; !ok {
		t.Fatal("set_whiteboard handler should remain registered")
	}
}

func TestRegistryOmitsSetVolumeTool(t *testing.T) {
	client := switchbot.NewSwitchbotClient("token", "secret", `{"hub2":"hub-device"}`)
	reg := New(Config{SwitchBotClient: client})

	if _, ok := reg.DefinitionByName("set_volume"); ok {
		t.Fatal("set_volume should not be registered")
	}
	if _, ok := reg.Handlers()["set_volume"]; ok {
		t.Fatal("set_volume handler should not be registered")
	}
}

func TestRegistryOmitsScheduleTimerTool(t *testing.T) {
	client := switchbot.NewSwitchbotClient("token", "secret", `{"hub2":"hub-device"}`)
	reg := New(Config{SwitchBotClient: client})

	if _, ok := reg.DefinitionByName("schedule_timer"); ok {
		t.Fatal("schedule_timer should not be registered")
	}
	if _, ok := reg.Handlers()["schedule_timer"]; ok {
		t.Fatal("schedule_timer handler should not be registered")
	}
}

func assertToolMode(t *testing.T, reg *Registry, name, want string) {
	t.Helper()
	modes := reg.ToolModes()
	if modes[name] != want {
		t.Fatalf("ToolModes()[%q] = %q, want %q", name, modes[name], want)
	}
	def, ok := reg.DefinitionByName(name)
	if !ok {
		t.Fatalf("%s should be registered", name)
	}
	if def["x_tool_mode"] != want {
		t.Fatalf("DefinitionByName(%q)[x_tool_mode] = %v, want %q", name, def["x_tool_mode"], want)
	}
}
