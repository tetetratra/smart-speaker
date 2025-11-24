package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"smart-speaker/internal/app"
	"smart-speaker/internal/components/audioplayer"
	"smart-speaker/internal/components/conversationstarter"
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
}

type stages struct {
	input    *graph.Stage
	text     *graph.Stage
	starter  *graph.Stage
	realtime *graph.Stage
	printer  *graph.Stage
	player   *graph.Stage
	tool     *graph.Stage
}

func (s stages) close() {
	for _, st := range []*graph.Stage{s.input, s.text, s.starter, s.realtime, s.printer, s.player, s.tool} {
		if st != nil {
			st.Close()
		}
	}
}

func buildStages(ctx context.Context, cfg app.Config) (stages, error) {
	input, err := buildInputStage(cfg)
	if err != nil {
		return stages{}, fmt.Errorf("failed to init input stage: %w", err)
	}
	text := textinput.NewStage()
	realtime, err := realtimeapi.NewStage(ctx, realtimeapi.Config{
		APIKey:             cfg.APIKey,
		Model:              cfg.Model,
		TranscriptionModel: cfg.TranscriptionModel,
		Instructions:       cfg.SystemPrompt,
		Voice:              cfg.Voice,
		DebugPrintMsgType:  cfg.Debug.PrintMsgType,
		DebugDumpResponses: cfg.Debug.DumpResponses,
	})
	if err != nil {
		input.Close()
		return stages{}, fmt.Errorf("failed to init realtime stage: %w", err)
	}
	printerStage := printer.NewStage()
	var starter *graph.Stage
	playerStage, err := audioplayer.NewStage()
	if err != nil {
		input.Close()
		text.Close()
		if starter != nil {
			starter.Close()
		}
		realtime.Close()
		printerStage.Close()
		return stages{}, fmt.Errorf("failed to init audio player stage: %w", err)
	}
	switchBotTool := toolcaller.NewSwitchBotTool(cfg.SwitchBot.Token, cfg.SwitchBot.Secret, cfg.SwitchBot.DeviceMap)
	tools := map[string]toolcaller.Tool{
		switchBotTool.Name(): switchBotTool,
	}
	toolStage := toolcaller.NewStage(tools)
	if cfg.AutoPromptInterval > 0 {
		starter = conversationstarter.NewStage(cfg.AutoPromptInterval, cfg.AutoPromptMessage)
	}
	return stages{
		input:    input,
		text:     text,
		starter:  starter,
		realtime: realtime,
		printer:  printerStage,
		player:   playerStage,
		tool:     toolStage,
	}, nil
}

func buildInputStage(cfg app.Config) (*graph.Stage, error) {
	if cfg.InputVoicePath != "" {
		return filereader.NewStage(cfg.InputVoicePath)
	}
	return micreader.NewStage()
}

func wireGraph(g *graph.Graph, st stages) {
	add := func(stage *graph.Stage) *graph.Node {
		if stage == nil {
			return nil
		}
		return g.AddNode(stage)
	}

	realtimeNode := add(st.realtime)
	if realtimeNode == nil {
		return
	}
	if node := add(st.input); node != nil {
		g.Connect(node, realtimeNode)
	}
	if node := add(st.text); node != nil {
		g.Connect(node, realtimeNode)
	}
	if node := add(st.starter); node != nil {
		g.Connect(node, realtimeNode)
	}
	if node := add(st.printer); node != nil {
		g.Connect(realtimeNode, node)
	}
	if node := add(st.player); node != nil {
		g.Connect(realtimeNode, node)
	}
	if node := add(st.tool); node != nil {
		toolNode := node
		g.Connect(realtimeNode, toolNode)
		g.Connect(toolNode, realtimeNode)
	}
}
