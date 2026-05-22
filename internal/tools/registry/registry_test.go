package registry

import (
	"testing"

	"smart-speaker/internal/tools/functions/switchbot"
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
	if _, ok := reg.DefinitionByName("hub2_get_environment"); !ok {
		t.Fatal("hub2_get_environment should be registered")
	}
	if _, ok := reg.DefinitionByName("set_whiteboard"); !ok {
		t.Fatal("set_whiteboard should be registered")
	}
	if _, ok := reg.Handlers()["set_whiteboard"]; !ok {
		t.Fatal("set_whiteboard handler should be registered")
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
}

func TestRegistryOmitsSceneToolWithoutScenes(t *testing.T) {
	client := switchbot.NewSwitchbotClient("token", "secret", `{"hub2":"hub-device"}`)
	reg := New(Config{SwitchBotClient: client})

	if _, ok := reg.DefinitionByName("switchbot_execute_scene"); ok {
		t.Fatal("switchbot_execute_scene should not be registered")
	}
	if _, ok := reg.DefinitionByName("hub2_get_environment"); !ok {
		t.Fatal("hub2_get_environment should be registered")
	}
}

func TestRegistryOmitsSwitchBotToolsWithoutCredentials(t *testing.T) {
	reg := New(Config{})

	for _, name := range []string{"switchbot_execute_scene", "hub2_get_environment"} {
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
	for _, name := range []string{"google_calendar_list", "google_calendar_create", "google_calendar_update"} {
		if _, ok := reg.DefinitionByName(name); !ok {
			t.Fatalf("%s should be registered", name)
		}
		if _, ok := reg.Handlers()[name]; !ok {
			t.Fatalf("%s handler should be registered", name)
		}
	}
}

func TestRegistryOmitsWebSearchTool(t *testing.T) {
	reg := New(Config{})

	if _, ok := reg.DefinitionByName("web_search"); ok {
		t.Fatal("web_search should not be registered")
	}
	if _, ok := reg.Handlers()["web_search"]; ok {
		t.Fatal("web_search handler should not be registered")
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
