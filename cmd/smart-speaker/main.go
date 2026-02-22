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
	"time"

	"smart-speaker/internal/app"
	"smart-speaker/internal/components/conversation"
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

	setLocalTimeZone()

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

func setLocalTimeZone() {
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		log.Printf("failed to load timezone: %v", err)
		return
	}
	time.Local = loc
}

func ensureGoogleCalendarToken() {
	if _, err := googlecalendar.LoadToken(); err == nil {
		return
	}
	log.Println("google oauth token not found. open /oauth/google/start to authenticate")
}

type stages struct {
	wsserver  *graph.Stage
	responses *graph.Stage
	tts       *graph.Stage
	chat      *graph.Stage
	rtc       *graph.Stage
	tool      *graph.Stage
	conv      *graph.Stage
	reset     *graph.Stage
}

func (s stages) close() {
	for _, st := range []*graph.Stage{s.wsserver, s.responses, s.tts, s.chat, s.rtc, s.tool, s.conv, s.reset} {
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
	if serverStage != nil {
		serverStage.Name = "wsserver"
	}
	if chatStage != nil {
		chatStage.Name = "wschat"
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
	if ttsStage != nil {
		ttsStage.Name = "tts"
	}
	convStage := conversation.NewStage(conversation.Config{
		LogPath: "data/conversation.jsonl",
	})
	if convStage != nil {
		convStage.Name = "conversation"
	}

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
	if resetStage != nil {
		resetStage.Name = "reset"
	}
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
	if responsesStage != nil {
		responsesStage.Name = "responsesapi"
	}
	toolStage := toolcaller.NewStage(toolRegistry.Handlers())
	if toolStage != nil {
		toolStage.Name = "toolcaller"
	}
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
	if rtcStage != nil {
		rtcStage.Name = "rtc"
	}
	return stages{
		wsserver:  serverStage,
		responses: responsesStage,
		tts:       ttsStage,
		chat:      chatStage,
		rtc:       rtcStage,
		tool:      toolStage,
		conv:      convStage,
		reset:     resetStage,
	}, nil
}

func buildWSStages(cfg app.Config) (*graph.Stage, *graph.Stage, error) {
	mux := http.NewServeMux()
	registerWebUI(mux, cfg.WebDistDir)
	googlecalendar.RegisterHTTPHandlers(mux)
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
	convNode := add(st.conv)
	resetNode := add(st.reset)
	ttsNode := add(st.tts)
	rtcNode := add(st.rtc)
	chatNode := add(st.chat)

	if resetNode != nil {
		if responsesNode != nil {
			g.Connect(resetNode, responsesNode)
		}
	}
	if resetNode != nil && convNode != nil {
		g.Connect(resetNode, convNode)
	}
	if responsesNode != nil && chatNode != nil {
		g.Connect(responsesNode, chatNode)
	}
	if convNode != nil && responsesNode != nil {
		g.Connect(convNode, responsesNode)
		g.Connect(responsesNode, convNode)
	}
	if ttsNode != nil {
		if convNode != nil {
			g.Connect(convNode, ttsNode)
			g.Connect(ttsNode, convNode)
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
	if toolNode != nil && convNode != nil {
		g.Connect(toolNode, convNode)
	}
	if chatNode != nil {
		if convNode != nil {
			g.Connect(chatNode, convNode)
			g.Connect(convNode, chatNode)
		}
		if rtcNode != nil {
			g.Connect(chatNode, rtcNode)
			g.Connect(rtcNode, chatNode)
		}
		if resetNode != nil {
			g.Connect(resetNode, chatNode)
			g.Connect(chatNode, resetNode)
		}
		if toolNode != nil {
			g.Connect(toolNode, chatNode)
		}
	}
	if rtcNode != nil && responsesNode != nil {
		g.Connect(rtcNode, responsesNode)
	}
	if convNode != nil && rtcNode != nil {
		g.Connect(convNode, rtcNode)
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
		cleanPath := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		if cleanPath == "/" {
			http.ServeFile(w, r, indexPath)
			return
		}
		targetPath := filepath.Join(absDir, strings.TrimPrefix(cleanPath, "/"))
		if info, err := os.Stat(targetPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		// assets や拡張子付きの静的ファイルは SPA フォールバックの対象外にする。
		if strings.HasPrefix(cleanPath, "/assets/") || cleanPath == "/assets" || path.Ext(cleanPath) != "" {
			http.NotFound(w, r)
			return
		}

		// SPA ルーティング向けに index.html を直接返す（URL書き換えしない）。
		http.ServeFile(w, r, indexPath)
	}))
}
