package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/tetetratra/smart-speaker/internal/app"
	"github.com/tetetratra/smart-speaker/internal/components/wschat"
	"github.com/tetetratra/smart-speaker/internal/graph"
	oauthgooglecalendar "github.com/tetetratra/smart-speaker/internal/oauth/googlecalendar"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	"github.com/tetetratra/smart-speaker/internal/states/userlocation"
)

func buildHTTPServer(cfg app.Config, memoryStore *memorystate.Store, services runtimeServices) (*http.Server, *graph.Stage, error) {
	mux := http.NewServeMux()
	registerMemoryAPI(mux, memoryStore, &manualMemoryService{store: memoryStore})
	registerSwitchBotSceneAPI(mux, buildSwitchBotClient(cfg.SwitchBot))
	registerLocationAPI(mux, &userlocation.Store{})
	registerTTSAPI(mux, services.TTS)
	registerWakeModeAPI(mux, services.WakeMode)
	registerToolHistoryAPI(mux, services.ToolHistory)
	registerAutomationsAPI(mux, services.Automations)
	registerAgentEventsAPI(mux, services.ClaudeCode, cfg.AgentBridge.Token)
	registerWebUI(mux, cfg.WebDistDir)
	oauthgooglecalendar.RegisterHTTPHandlers(mux)
	server := &http.Server{
		Addr:    cfg.WSAddr,
		Handler: mux,
	}
	chat := wschat.NewStage(mux, wschat.Config{})
	return server, chat, nil
}

func closeHTTPServer(server *http.Server) {
	if server != nil {
		_ = server.Close()
	}
}

func runHTTPServer(server *http.Server) {
	if server == nil {
		return
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http server listen error: %v", err)
		}
	}()
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("http: encode response: %v", err)
	}
}
