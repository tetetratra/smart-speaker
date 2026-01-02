package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"smart-speaker/internal/app"
	"smart-speaker/internal/components/printer"
	"smart-speaker/internal/components/realtimeapi"
	"smart-speaker/internal/components/responsesapi"
	"smart-speaker/internal/components/toolcaller"
	"smart-speaker/internal/components/tts"
	"smart-speaker/internal/components/turnmanager"
	"smart-speaker/internal/components/wsaudio"
	"smart-speaker/internal/components/wschat"
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

	log.Printf("wsinput downstream nil? %v", stages.input.Downstream == nil)
	log.Printf("realtime upstream nil? %v", stages.realtime.Upstream == nil)
	log.Printf("main ctx err: %v", ctx.Err())

	if err := g.Run(ctx); err != nil {
		log.Fatalf("graph run error: %v", err)
	}
}

type stages struct {
	input     *graph.Stage
	realtime  *graph.Stage
	turn      *graph.Stage
	responses *graph.Stage
	printer   *graph.Stage
	player    *graph.Stage
	tts       *graph.Stage
	chat      *graph.Stage
	tool      *graph.Stage
}

func (s stages) close() {
	for _, st := range []*graph.Stage{s.input, s.realtime, s.turn, s.responses, s.printer, s.player, s.tts, s.chat, s.tool} {
		if st != nil {
			st.Close()
		}
	}
}

func buildStages(ctx context.Context, cfg app.Config) (stages, error) {
	inStage, outStage, chatStage, err := buildWSStages(cfg)
	if err != nil {
		return stages{}, fmt.Errorf("failed to init ws stages: %w", err)
	}
	ttsStage, err := tts.NewStage(tts.Config{
		APIKey: cfg.ElevenLabs.APIKey,
		Voice:  cfg.ElevenLabs.VoiceID,
		Model:  cfg.ElevenLabs.Model,
	})
	if err != nil {
		inStage.Close()
		outStage.Close()
		return stages{}, fmt.Errorf("failed to init elevenlabs stage: %w", err)
	}
	realtime, err := realtimeapi.NewStage(ctx, realtimeapi.Config{
		APIKey:             cfg.APIKey,
		Model:              cfg.Model,
		TranscriptionModel: cfg.TranscriptionModel,
		Instructions:       cfg.SystemPrompt,
		Voice:              cfg.Voice,
		Modalities:         []string{"text"},
		DebugPrintMsgType:  cfg.Debug.PrintMsgType,
		DebugDumpResponses: cfg.Debug.DumpResponses,
	})
	if err != nil {
		inStage.Close()
		outStage.Close()
		if ttsStage != nil {
			ttsStage.Close()
		}
		return stages{}, fmt.Errorf("failed to init realtime stage: %w", err)
	}
	printerStage := printer.NewStage()
	turnStage := turnmanager.NewStage()
	responsesStage, err := responsesapi.NewStage(responsesapi.Config{
		APIKey:       cfg.APIKey,
		Model:        cfg.ResponsesModel,
		Instructions: cfg.SystemPrompt,
	})
	if err != nil {
		inStage.Close()
		outStage.Close()
		if ttsStage != nil {
			ttsStage.Close()
		}
		realtime.Close()
		return stages{}, fmt.Errorf("failed to init responses stage: %w", err)
	}
	switchBotTool := toolcaller.NewSwitchBotTool(cfg.SwitchBot.Token, cfg.SwitchBot.Secret, cfg.SwitchBot.DeviceMap)
	subAITool := toolcaller.NewSubAITool(cfg.APIKey)
	timerTool := toolcaller.NewTimerTool()
	tools := map[string]toolcaller.Tool{}
	if switchBotTool != nil {
		tools[switchBotTool.Name()] = switchBotTool
	}
	if subAITool != nil {
		tools[subAITool.Name()] = subAITool
	}
	if timerTool != nil {
		tools[timerTool.Name()] = timerTool
	}
	toolStage := toolcaller.NewStage(tools)
	return stages{
		input:     inStage,
		realtime:  realtime,
		turn:      turnStage,
		responses: responsesStage,
		printer:   printerStage,
		player:    outStage,
		tts:       ttsStage,
		chat:      chatStage,
		tool:      toolStage,
	}, nil
}

func buildWSStages(cfg app.Config) (*graph.Stage, *graph.Stage, *graph.Stage, error) {
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    cfg.WSAddr,
		Handler: mux,
	}
	in, out := wsaudio.NewStages(server, mux)
	chat := wschat.NewStage(mux)
	return in, out, chat, nil
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
	turnNode := add(st.turn)
	responsesNode := add(st.responses)
	if node := add(st.input); node != nil {
		g.Connect(node, realtimeNode)
	}
	if node := add(st.printer); node != nil {
		g.Connect(realtimeNode, node)
		if responsesNode != nil {
			g.Connect(responsesNode, node)
		}
	}
	if turnNode != nil {
		g.Connect(realtimeNode, turnNode)
	}
	if responsesNode != nil && turnNode != nil {
		g.Connect(turnNode, responsesNode)
	}
	if node := add(st.tts); node != nil {
		if responsesNode != nil {
			g.Connect(responsesNode, node)
		}
		if player := add(st.player); player != nil {
			g.Connect(node, player)
		}
	}
	var toolNode *graph.Node
	if node := add(st.tool); node != nil {
		toolNode = node
		if responsesNode != nil {
			g.Connect(responsesNode, toolNode)
			g.Connect(toolNode, responsesNode)
		}
	}
	if node := add(st.chat); node != nil {
		g.Connect(realtimeNode, node)
		if responsesNode != nil {
			g.Connect(responsesNode, node)
			g.Connect(node, responsesNode)
		}
		if toolNode != nil {
			g.Connect(toolNode, node)
		}
	}
}
