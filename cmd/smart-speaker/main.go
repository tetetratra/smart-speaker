package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
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
	tts       *graph.Stage
	chat      *graph.Stage
	rtc       *graph.Stage
	tool      *graph.Stage
	proactive *graph.Stage
	reset     *graph.Stage
	followup  *graph.Stage
}

func (s stages) close() {
	for _, st := range []*graph.Stage{s.wsserver, s.responses, s.printer, s.tts, s.chat, s.rtc, s.tool, s.proactive, s.reset, s.followup} {
		if st != nil {
			st.Close()
		}
	}
}

func buildStages(cfg app.Config) (stages, error) {
	serverStage, chatStage, err := buildWSStages(cfg)
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
		if ttsStage != nil {
			ttsStage.Close()
		}
		return stages{}, fmt.Errorf("failed to init responses stage: %w", err)
	}
	toolStage := toolcaller.NewStage(toolRegistry.Handlers())
	rtcStage, err := rtc.NewStage(rtc.Config{
		IceHostIPs: cfg.RTCIceHostIPs,
	})
	if err != nil {
		serverStage.Close()
		if ttsStage != nil {
			ttsStage.Close()
		}
		return stages{}, fmt.Errorf("failed to init rtc stage: %w", err)
	}
	return stages{
		wsserver:  serverStage,
		responses: responsesStage,
		printer:   printerStage,
		tts:       ttsStage,
		chat:      chatStage,
		rtc:       rtcStage,
		tool:      toolStage,
		proactive: proactiveStage,
		reset:     resetStage,
		followup:  followupStage,
	}, nil
}

func buildWSStages(cfg app.Config) (*graph.Stage, *graph.Stage, error) {
	mux := http.NewServeMux()
	registerWebUI(mux, cfg.WebDistDir)
	server := &http.Server{
		Addr:    cfg.WSAddr,
		Handler: mux,
	}
	serverStage := wsserver.NewStage(server)
	chat := wschat.NewStage(mux)
	return serverStage, chat, nil
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

func registerWebUI(mux *http.ServeMux, distDir string) {
	if distDir == "" {
		distDir = "web/dist"
	}
	absDir, err := filepath.Abs(distDir)
	if err != nil {
		log.Printf("web ui: invalid dist dir: %v", err)
		return
	}
	info, err := os.Stat(absDir)
	if err != nil || !info.IsDir() {
		log.Printf("web ui: dist dir not found: %s", absDir)
		return
	}
	indexPath := filepath.Join(absDir, "index.html")
	if info, err := os.Stat(indexPath); err != nil || info.IsDir() {
		log.Printf("web ui: index.html not found: %s", indexPath)
		return
	}
	log.Printf("web ui: serve %s", absDir)

	fileServer := http.FileServer(http.Dir(absDir))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		cleanPath := path.Clean(r.URL.Path)
		if cleanPath == "/" {
			cleanPath = "/index.html"
		}
		targetPath := filepath.Join(absDir, strings.TrimPrefix(cleanPath, "/"))
		if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		clone := *r
		urlCopy := *r.URL
		clone.URL = &urlCopy
		clone.URL.Path = "/index.html"
		fileServer.ServeHTTP(w, &clone)
	}))
}
