package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"smart-speaker/internal/app"
	"smart-speaker/internal/components/filereader"
	"smart-speaker/internal/components/micreader"
	"smart-speaker/internal/components/printer"
	"smart-speaker/internal/components/realtimeapi"
	"smart-speaker/internal/graph"
)

func main() {
	log.SetFlags(0)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := app.LoadConfig("system_prompt.txt")

	var voiceStage graph.Stage
	if cfg.InputVoicePath != "" {
		stage, err := filereader.NewStage(cfg.InputVoicePath)
		if err != nil {
			log.Fatalf("failed to init file reader stage: %v", err)
		}
		voiceStage = stage
	}
	micStage, err := micreader.NewStage()
	if err != nil {
		log.Fatalf("failed to init mic reader stage: %v", err)
	}

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

	printerStage := printer.NewPrinter()

	g := graph.New()
	defer g.Close()
	var voiceNode *graph.Node
	if voiceStage != nil {
		voiceNode = g.AddNode(voiceStage)
	}
	micNode := g.AddNode(micStage)
	realtimeNode := g.AddNode(realtimeStage)
	printerNode := g.AddNode(printerStage)

	if voiceNode != nil {
		g.Connect(voiceNode, realtimeNode)
	}
	g.Connect(micNode, realtimeNode)
	g.Connect(realtimeNode, printerNode)

	if err := g.Run(ctx); err != nil {
		log.Fatalf("graph run error: %v", err)
	}

	fmt.Println("🎙 音声ストリーミングを開始しました。Ctrl+C で終了します。")
}
