package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"

	"smart-speaker/internal/app"
	"smart-speaker/internal/components/followup"
	"smart-speaker/internal/components/printer"
	"smart-speaker/internal/components/proactive"
	"smart-speaker/internal/components/reset"
	"smart-speaker/internal/components/responsesapi"
	"smart-speaker/internal/components/rtc"
	"smart-speaker/internal/components/toolcaller"
	"smart-speaker/internal/components/tts"
	"smart-speaker/internal/components/wsaudio"
	"smart-speaker/internal/components/wschat"
	"smart-speaker/internal/components/wsserver"
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

	stages, err := buildStages(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer stages.close()

	g := graph.New()
	defer g.Close()

	wireGraph(g, stages)

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
	wsserver  *graph.Stage
	responses *graph.Stage
	printer   *graph.Stage
	player    *graph.Stage
	tts       *graph.Stage
	chat      *graph.Stage
	rtc       *graph.Stage
	tool      *graph.Stage
	proactive *graph.Stage
	reset     *graph.Stage
	followup  *graph.Stage
}

func (s stages) close() {
	for _, st := range []*graph.Stage{s.wsserver, s.responses, s.printer, s.player, s.tts, s.chat, s.rtc, s.tool, s.proactive, s.reset, s.followup} {
		if st != nil {
			st.Close()
		}
	}
}

func buildStages(cfg app.Config) (stages, error) {
	serverStage, outStage, chatStage, err := buildWSStages(cfg)
	if err != nil {
		return stages{}, fmt.Errorf("failed to init ws stages: %w", err)
	}
	ttsStage, err := tts.NewStage(tts.Config{
		APIKey: cfg.ElevenLabs.APIKey,
		Voice:  cfg.ElevenLabs.VoiceID,
		Model:  cfg.ElevenLabs.Model,
	})
	if err != nil {
		serverStage.Close()
		outStage.Close()
		return stages{}, fmt.Errorf("failed to init elevenlabs stage: %w", err)
	}
	printerStage := printer.NewStage()
	proactiveStage := proactive.NewStage()
	followupStage := followup.NewStage()

	toolRegistry := registry.New(registry.Config{
		SwitchBotToken:     cfg.SwitchBot.Token,
		SwitchBotSecret:    cfg.SwitchBot.Secret,
		SwitchBotDeviceMap: cfg.SwitchBot.DeviceMap,
	})
	writeDiaryTools := []any{}
	if def, ok := toolRegistry.DefinitionByName("write_diary"); ok {
		writeDiaryTools = append(writeDiaryTools, def)
	}
	resetStage := reset.NewStage(reset.Config{WriteDiaryTools: writeDiaryTools})
	responsesStage, err := responsesapi.NewStage(responsesapi.Config{
		APIKey:       cfg.APIKey,
		Model:        cfg.ResponsesModel,
		Instructions: cfg.SystemPrompt,
		Tools:        toolRegistry.DefinitionsExcluding("write_diary"),
	})
	if err != nil {
		serverStage.Close()
		outStage.Close()
		if ttsStage != nil {
			ttsStage.Close()
		}
		return stages{}, fmt.Errorf("failed to init responses stage: %w", err)
	}
	toolStage := toolcaller.NewStage(toolRegistry.Handlers())
	rtcStage, err := rtc.NewStage(rtc.Config{
		ModelPath:  cfg.Vosk.ModelPath,
		IceHostIPs: cfg.RTCIceHostIPs,
		IcePortMin: cfg.RTCIcePortMin,
		IcePortMax: cfg.RTCIcePortMax,
	})
	if err != nil {
		serverStage.Close()
		outStage.Close()
		if ttsStage != nil {
			ttsStage.Close()
		}
		return stages{}, fmt.Errorf("failed to init rtc stage: %w", err)
	}
	return stages{
		wsserver:  serverStage,
		responses: responsesStage,
		printer:   printerStage,
		player:    outStage,
		tts:       ttsStage,
		chat:      chatStage,
		rtc:       rtcStage,
		tool:      toolStage,
		proactive: proactiveStage,
		reset:     resetStage,
		followup:  followupStage,
	}, nil
}

func buildWSStages(cfg app.Config) (*graph.Stage, *graph.Stage, *graph.Stage, error) {
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    cfg.WSAddr,
		Handler: mux,
	}
	serverStage := wsserver.NewStage(server)
	out := wsaudio.NewStage(mux)
	chat := wschat.NewStage(mux)
	return serverStage, out, chat, nil
}

func wireGraph(g *graph.Graph, st stages) {
	add := func(stage *graph.Stage) *graph.Node {
		if stage == nil {
			return nil
		}
		return g.AddNode(stage)
	}

	if node := add(st.wsserver); node == nil {
		return
	}
	responsesNode := add(st.responses)
	proactiveNode := add(st.proactive)
	resetNode := add(st.reset)
	followupNode := add(st.followup)
	printerNode := add(st.printer)
	ttsNode := add(st.tts)
	playerNode := add(st.player)
	rtcNode := add(st.rtc)
	chatNode := add(st.chat)

	if printerNode != nil {
		if responsesNode != nil {
			g.Connect(responsesNode, printerNode)
		}
		if proactiveNode != nil {
			g.Connect(proactiveNode, printerNode)
		}
		if resetNode != nil {
			g.Connect(resetNode, printerNode)
		}
	}
	if proactiveNode != nil {
		if responsesNode != nil {
			g.Connect(proactiveNode, responsesNode)
		}
	}
	if resetNode != nil {
		if responsesNode != nil {
			g.Connect(resetNode, responsesNode)
		}
	}
	if followupNode != nil {
		if responsesNode != nil {
			g.Connect(responsesNode, followupNode)
			g.Connect(followupNode, responsesNode)
		}
	}
	if ttsNode != nil {
		if responsesNode != nil {
			g.Connect(responsesNode, ttsNode)
		}
		if followupNode != nil {
			g.Connect(ttsNode, followupNode)
		}
		if rtcNode != nil {
			g.Connect(ttsNode, rtcNode)
		}
		if playerNode != nil {
			g.Connect(ttsNode, playerNode)
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
	if chatNode != nil {
		if responsesNode != nil {
			g.Connect(responsesNode, chatNode)
			g.Connect(chatNode, responsesNode)
		}
		if rtcNode != nil {
			g.Connect(chatNode, rtcNode)
			g.Connect(rtcNode, chatNode)
		}
		if proactiveNode != nil {
			g.Connect(proactiveNode, chatNode)
		}
		if resetNode != nil {
			g.Connect(resetNode, chatNode)
			g.Connect(chatNode, resetNode)
		}
		if followupNode != nil {
			g.Connect(chatNode, followupNode)
		}
		if toolNode != nil {
			g.Connect(toolNode, chatNode)
		}
	}
	if rtcNode != nil && responsesNode != nil {
		g.Connect(rtcNode, responsesNode)
	}
}
