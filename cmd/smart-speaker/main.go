package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"smart-speaker/internal/app"
	"smart-speaker/internal/components/printer"
	"smart-speaker/internal/components/realtimeapi"
	"smart-speaker/internal/components/voice"
	"smart-speaker/internal/graph"
)

func main() {
	log.SetFlags(0)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := app.LoadConfig("system_prompt.txt")

	voiceStage, err := voice.NewStage(cfg.InputVoicePath)
	if err != nil {
		log.Fatalf("failed to init voice stage: %v", err)
	}
	defer voiceStage.Close()

	realtimeStage, err := realtimeapi.NewStage(ctx, realtimeapi.Config{
		APIKey:             cfg.APIKey,
		Model:              cfg.Model,
		TranscriptionModel: cfg.TranscriptionModel,
		Instructions:       cfg.SystemPrompt,
	})
	if err != nil {
		log.Fatalf("failed to init realtime stage: %v", err)
	}
	defer realtimeStage.Close()

	printerStage := printer.NewStage()
	defer printerStage.Close()

	g := graph.New()
	g.Add(voiceStage)
	g.Add(realtimeStage)
	g.Add(printerStage)

	if err := g.Run(ctx); err != nil {
		log.Fatalf("graph run error: %v", err)
	}

	fmt.Println("🎙 音声ストリーミングを開始しました。Ctrl+C で終了します。")
	g.Close()
}
