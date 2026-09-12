package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/tetetratra/smart-speaker/internal/app"
	"github.com/tetetratra/smart-speaker/internal/components/wschat"
	"github.com/tetetratra/smart-speaker/internal/graph"
	memoryhook "github.com/tetetratra/smart-speaker/internal/hooks/memory"
	oauthgooglecalendar "github.com/tetetratra/smart-speaker/internal/oauth/googlecalendar"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
)

func buildHTTPServer(cfg app.Config, memoryStore *memorystate.Store) (*http.Server, *graph.Stage, error) {
	mux := http.NewServeMux()
	memoryEmbedder, err := memoryhook.NewEmbeddingClient(memoryhook.EmbeddingClientConfig{
		BaseURL: cfg.Memory.EmbeddingBaseURL,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init memory api embedder: %w", err)
	}
	memoryTagger, err := memoryhook.NewOpenAIClient(memoryhook.OpenAIClientConfig{
		APIKey:  cfg.APIKey,
		Model:   cfg.Memory.Model,
		MaxTags: cfg.Memory.MaxTags,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to init memory api tagger: %w", err)
	}
	registerMemoryAPI(mux, memoryStore, &manualMemoryService{
		store:                  memoryStore,
		tagger:                 memoryTagger,
		embedder:               memoryEmbedder,
		duplicateMinSimilarity: cfg.Memory.DuplicateMinSimilarity,
	})
	registerSwitchBotSceneAPI(mux, buildSwitchBotClient(cfg.SwitchBot))
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
