package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"smart-speaker/internal/app"
	"smart-speaker/internal/components/realtimeapi"
	"smart-speaker/internal/pipeline"
)

func main() {
	log.SetFlags(0)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := app.LoadConfig("system_prompt.txt")

	voiceStage, err := pipeline.NewVoiceStage(cfg.InputVoicePath)
	if err != nil {
		log.Fatalf("failed to initialize voice stage: %v", err)
	}

	realtimeStage, err := pipeline.NewRealtimeStage(ctx, realtimeapi.Config{
		APIKey:             cfg.APIKey,
		Model:              cfg.Model,
		TranscriptionModel: cfg.TranscriptionModel,
		Instructions:       cfg.SystemPrompt,
	})
	if err != nil {
		log.Fatalf("failed to initialize realtime stage: %v", err)
	}

	printerStage := pipeline.NewPrinterStage()

	pipe := pipeline.New(voiceStage, realtimeStage, printerStage)
	defer pipe.Close()

	if err := pipe.Run(ctx); err != nil {
		log.Fatalf("pipeline run error: %v", err)
	}

	fmt.Println("🎙 音声ストリーミングを開始しました。Ctrl+C で終了します。")
	<-ctx.Done()
}
