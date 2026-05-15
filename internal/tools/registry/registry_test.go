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
