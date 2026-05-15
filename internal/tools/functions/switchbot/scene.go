package switchbot

import (
	"context"
	"fmt"
	"strings"

	"smart-speaker/internal/tools"
)

const sceneToolName = "switchbot_execute_scene"

type SceneTool struct {
	client       *Client
	ctx          context.Context
	scenesByName map[string]Scene
	sceneNames   []string
}

func NewScene(client *Client, scenes []Scene) *SceneTool {
	if client == nil || len(scenes) == 0 {
		return nil
	}
	scenesByName := make(map[string]Scene, len(scenes))
	sceneNames := make([]string, 0, len(scenes))
	for _, scene := range scenes {
		sceneName := strings.TrimSpace(scene.SceneName)
		sceneID := strings.TrimSpace(scene.SceneID)
		if sceneName == "" || sceneID == "" {
			continue
		}
		scenesByName[sceneName] = Scene{SceneID: sceneID, SceneName: sceneName}
		sceneNames = append(sceneNames, sceneName)
	}
	if len(scenesByName) == 0 {
		return nil
	}
	return &SceneTool{
		client:       client,
		scenesByName: scenesByName,
		sceneNames:   sceneNames,
	}
}

func (t *SceneTool) Name() string { return sceneToolName }

func (t *SceneTool) SetContext(ctx context.Context) {
	t.ctx = ctx
}

func (t *SceneTool) Run(args map[string]any) (map[string]any, error) {
	if t.client == nil {
		return nil, errNotConfigured
	}
	sceneName := strings.TrimSpace(asString(args["scene_name"]))
	if sceneName == "" {
		return nil, fmt.Errorf("scene_name を指定してください。利用可能なシーン: %s", strings.Join(t.sceneNames, ", "))
	}
	scene, ok := t.scenesByName[sceneName]
	if !ok {
		return nil, fmt.Errorf("未登録のシーンです: %s。利用可能なシーン: %s", sceneName, strings.Join(t.sceneNames, ", "))
	}
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return t.client.ExecuteScene(ctx, scene.SceneID)
}

func (t *SceneTool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        sceneToolName,
		"description": fmt.Sprintf("SwitchBotに登録済みのシーンを実行します。scene_name は次のいずれかを完全一致で指定します: %s", strings.Join(t.sceneNames, ", ")),
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scene_name": map[string]any{
					"type":        "string",
					"description": "起動時に取得したSwitchBotシーン名",
				},
			},
			"required":             []string{"scene_name"},
			"additionalProperties": false,
		},
	}
}

var _ tools.Handler = (*SceneTool)(nil)
var _ tools.ContextAware = (*SceneTool)(nil)
var _ tools.DefinitionProvider = (*SceneTool)(nil)
