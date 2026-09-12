package main

import (
	"context"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/tetetratra/smart-speaker/internal/tools/functions/switchbot"
)

type switchBotSceneClient interface {
	ListScenes(context.Context) ([]switchbot.Scene, error)
	ExecuteScene(context.Context, string) (map[string]any, error)
}

type switchBotSceneListResponse struct {
	Scenes []switchBotSceneListItem `json:"scenes"`
}

type switchBotSceneListItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type switchBotSceneExecuteResponse struct {
	Result map[string]any `json:"result"`
}

func registerSwitchBotSceneAPI(mux *http.ServeMux, client switchBotSceneClient) {
	mux.HandleFunc("/api/switchbot/scenes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		handleSwitchBotSceneList(w, r, client)
	})
	mux.HandleFunc("/api/switchbot/scenes/", func(w http.ResponseWriter, r *http.Request) {
		sceneID := strings.TrimPrefix(path.Clean(r.URL.Path), "/api/switchbot/scenes/")
		sceneID = strings.TrimSuffix(sceneID, "/execute")
		if sceneID == "." || sceneID == "" || !strings.HasSuffix(path.Clean(r.URL.Path), "/execute") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		handleSwitchBotSceneExecute(w, r, client, sceneID)
	})
}

func handleSwitchBotSceneList(w http.ResponseWriter, r *http.Request, client switchBotSceneClient) {
	if client == nil {
		http.Error(w, "switchbot is unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	scenes, err := client.ListScenes(ctx)
	if err != nil {
		http.Error(w, "failed to list switchbot scenes", http.StatusBadGateway)
		return
	}
	items := make([]switchBotSceneListItem, 0, len(scenes))
	for _, scene := range scenes {
		sceneID := strings.TrimSpace(scene.SceneID)
		sceneName := strings.TrimSpace(scene.SceneName)
		if sceneID == "" || sceneName == "" {
			continue
		}
		items = append(items, switchBotSceneListItem{ID: sceneID, Name: sceneName})
	}
	writeJSON(w, http.StatusOK, switchBotSceneListResponse{Scenes: items})
}

func handleSwitchBotSceneExecute(w http.ResponseWriter, r *http.Request, client switchBotSceneClient, sceneID string) {
	if client == nil {
		http.Error(w, "switchbot is unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result, err := client.ExecuteScene(ctx, sceneID)
	if err != nil {
		http.Error(w, "failed to execute switchbot scene", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, switchBotSceneExecuteResponse{Result: result})
}
