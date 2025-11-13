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
	"smart-speaker/internal/components/textinput"
	"smart-speaker/internal/components/toolcaller"
	"smart-speaker/internal/graph"
)

func main() {
	log.SetFlags(0)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := app.LoadConfig("system_prompt.txt")

	stages, err := buildStages(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer stages.close()

	g := graph.New()
	defer g.Close()

	wireGraph(g, stages)

	if err := g.Run(ctx); err != nil {
		log.Fatalf("graph run error: %v", err)
	}

	fmt.Println("🎙 音声ストリーミングを開始しました。Ctrl+C で終了します。")
}

type stages struct {
	input    graph.Stage
	text     graph.Stage
	realtime graph.Stage
	printer  graph.Stage
	tool     graph.Stage
}

func (s stages) close() {
	if s.input != nil {
		s.input.Close()
	}
	if s.text != nil {
		s.text.Close()
	}
	if s.realtime != nil {
		s.realtime.Close()
	}
	if s.printer != nil {
		s.printer.Close()
	}
	if s.tool != nil {
		s.tool.Close()
	}
}

func buildStages(ctx context.Context, cfg app.Config) (stages, error) {
	input, err := buildInputStage(cfg)
	if err != nil {
		return stages{}, fmt.Errorf("failed to init input stage: %w", err)
	}
	text := textinput.NewStage(ctx)
	realtime, err := realtimeapi.NewStage(ctx, realtimeapi.Config{
		APIKey:             cfg.APIKey,
		Model:              cfg.Model,
		TranscriptionModel: cfg.TranscriptionModel,
		Instructions:       cfg.SystemPrompt,
	})
	if err != nil {
		input.Close()
		return stages{}, fmt.Errorf("failed to init realtime stage: %w", err)
	}
	printer := printer.NewPrinter()
	tool := toolcaller.NewStage()
	return stages{
		input:    input,
		text:     text,
		realtime: realtime,
		printer:  printer,
		tool:     tool,
	}, nil
}

func buildInputStage(cfg app.Config) (graph.Stage, error) {
	if cfg.InputVoicePath != "" {
		return filereader.NewStage(cfg.InputVoicePath)
	}
	return micreader.NewStage()
}

func wireGraph(g *graph.Graph, st stages) {
	inputNode := g.AddNode(st.input)
	textNode := g.AddNode(st.text)
	realtimeNode := g.AddNode(st.realtime)
	printerNode := g.AddNode(st.printer)
	toolNode := g.AddNode(st.tool)

	g.Connect(inputNode, realtimeNode)
	g.Connect(textNode, realtimeNode)
	g.Connect(realtimeNode, printerNode)
	g.Connect(realtimeNode, toolNode)
	g.Connect(toolNode, realtimeNode)
}
