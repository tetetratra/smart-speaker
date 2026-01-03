package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"smart-speaker/internal/app"
	"smart-speaker/internal/components/diaryreset"
	"smart-speaker/internal/components/diarywriter"
	"smart-speaker/internal/components/printer"
	"smart-speaker/internal/components/proactive"
	"smart-speaker/internal/components/realtimeapi"
	"smart-speaker/internal/components/responsesapi"
	"smart-speaker/internal/components/toolcaller"
	"smart-speaker/internal/components/tts"
	"smart-speaker/internal/components/turnmanager"
	"smart-speaker/internal/components/wsaudio"
	"smart-speaker/internal/components/wschat"
	"smart-speaker/internal/graph"
	"smart-speaker/internal/oauth/googlecalendar"
	"smart-speaker/internal/tools/registry"
)

func main() {
	log.SetFlags(0)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := app.LoadConfig("system_prompt.txt")

	ensureGoogleCalendarToken()

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

func ensureGoogleCalendarToken() {
	if _, err := googlecalendar.LoadToken(); err == nil {
		return
	}
	log.Println("google oauth token not found. starting auth flow.")
	if err := googlecalendar.StartAuthFlow(":3939"); err != nil {
		log.Printf("google oauth flow failed: %v", err)
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
	proactive *graph.Stage
	diary     *graph.Stage
	reset     *graph.Stage
}

func (s stages) close() {
	for _, st := range []*graph.Stage{s.input, s.realtime, s.turn, s.responses, s.printer, s.player, s.tts, s.chat, s.tool, s.proactive, s.diary, s.reset} {
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
	proactiveStage := proactive.NewStage()
	diaryStage := diarywriter.NewStage()
	resetStage := diaryreset.NewStage()

	toolRegistry := registry.New(registry.Config{
		SwitchBotToken:     cfg.SwitchBot.Token,
		SwitchBotSecret:    cfg.SwitchBot.Secret,
		SwitchBotDeviceMap: cfg.SwitchBot.DeviceMap,
	})
	responsesStage, err := responsesapi.NewStage(responsesapi.Config{
		APIKey:       cfg.APIKey,
		Model:        cfg.ResponsesModel,
		Instructions: cfg.SystemPrompt,
		Tools:        toolRegistry.Definitions(),
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
	toolStage := toolcaller.NewStage(toolRegistry.Handlers())
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
		proactive: proactiveStage,
		diary:     diaryStage,
		reset:     resetStage,
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
	proactiveNode := add(st.proactive)
	diaryNode := add(st.diary)
	_ = add(st.reset)
	if node := add(st.input); node != nil {
		g.Connect(node, realtimeNode)
	}
	if node := add(st.printer); node != nil {
		g.Connect(realtimeNode, node)
		if responsesNode != nil {
			g.Connect(responsesNode, node)
		}
		if proactiveNode != nil {
			g.Connect(proactiveNode, node)
		}
		if diaryNode != nil {
			g.Connect(diaryNode, node)
		}
	}
	if proactiveNode != nil {
		if responsesNode != nil {
			g.Connect(proactiveNode, responsesNode)
		}
	}
	if diaryNode != nil {
		if responsesNode != nil {
			g.Connect(diaryNode, responsesNode)
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
		if proactiveNode != nil {
			g.Connect(proactiveNode, node)
		}
		if diaryNode != nil {
			g.Connect(diaryNode, node)
		}
		if toolNode != nil {
			g.Connect(toolNode, node)
		}
	}
}
