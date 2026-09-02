package main

import (
	"context"
	"errors"
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

	"github.com/tetetratra/smart-speaker/internal/app"
	"github.com/tetetratra/smart-speaker/internal/components/conversationcommitter"
	"github.com/tetetratra/smart-speaker/internal/components/generationfilter"
	"github.com/tetetratra/smart-speaker/internal/components/interimstopper"
	"github.com/tetetratra/smart-speaker/internal/components/llm"
	"github.com/tetetratra/smart-speaker/internal/components/openaistt"
	"github.com/tetetratra/smart-speaker/internal/components/router"
	"github.com/tetetratra/smart-speaker/internal/components/rtcout"
	"github.com/tetetratra/smart-speaker/internal/components/rtcpeer"
	"github.com/tetetratra/smart-speaker/internal/components/rtcvad"
	"github.com/tetetratra/smart-speaker/internal/components/scheduler"
	"github.com/tetetratra/smart-speaker/internal/components/sessionactivate"
	"github.com/tetetratra/smart-speaker/internal/components/sessionreset"
	"github.com/tetetratra/smart-speaker/internal/components/stt"
	"github.com/tetetratra/smart-speaker/internal/components/toolcaller"
	"github.com/tetetratra/smart-speaker/internal/components/tts"
	"github.com/tetetratra/smart-speaker/internal/components/utterancebuffer"
	"github.com/tetetratra/smart-speaker/internal/components/wschat"
	"github.com/tetetratra/smart-speaker/internal/graph"
	oauthgooglecalendar "github.com/tetetratra/smart-speaker/internal/oauth/googlecalendar"
	"github.com/tetetratra/smart-speaker/internal/states/agentstatus"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	timerstate "github.com/tetetratra/smart-speaker/internal/states/timer"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/tools/functions/switchbot"
	timerfunc "github.com/tetetratra/smart-speaker/internal/tools/functions/timer"
	"github.com/tetetratra/smart-speaker/internal/tools/registry"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func main() {
	log.SetFlags(0)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	setLocalTimeZone()

	cfg := app.LoadConfig("system_prompt.txt")

	ensureGoogleCalendarToken()

	timerStore := timerstate.NewStore()

	server, chatStage, err := buildHTTPServer(cfg, timerStore)
	if err != nil {
		log.Fatal(err)
	}
	defer closeHTTPServer(server)

	stages, err := buildStages(cfg, chatStage, timerStore)
	if err != nil {
		log.Fatal(err)
	}
	defer closeStages(stages.all()...)

	g := graph.New()
	defer g.Close()

	wireGraph(g, stages)
	runHTTPServer(server)

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
	if _, err := oauthgooglecalendar.LoadToken(); err == nil {
		return
	}
	log.Println("google oauth token not found. open /oauth/google/start to authenticate")
}

func closeHTTPServer(server *http.Server) {
	if server != nil {
		_ = server.Close()
	}
}

func closeStages(stages ...*graph.Stage) {
	for _, st := range stages {
		if st != nil {
			st.Close()
		}
	}
}

type appStages struct {
	chat         *graph.Stage
	rtcpeer      *graph.Stage
	rtcvad       *graph.Stage
	stt          *graph.Stage
	interimStop  *graph.Stage
	rtcout       *graph.Stage
	utterance    *graph.Stage
	sessionReset *graph.Stage
	committer    *graph.Stage
	llm          *graph.Stage
	sessionAct   *graph.Stage
	filterLLM    *graph.Stage
	tts          *graph.Stage
	filterTTS    *graph.Stage
	scheduler    *graph.Stage
	filterSched  *graph.Stage
	router       *graph.Stage
	tool         *graph.Stage
}

func (s appStages) all() []*graph.Stage {
	return []*graph.Stage{
		s.chat,
		s.rtcpeer,
		s.rtcvad,
		s.stt,
		s.interimStop,
		s.rtcout,
		s.utterance,
		s.sessionReset,
		s.committer,
		s.llm,
		s.sessionAct,
		s.filterLLM,
		s.tts,
		s.filterTTS,
		s.scheduler,
		s.filterSched,
		s.router,
		s.tool,
	}
}

func buildStages(cfg app.Config, chatStage *graph.Stage, timerStore *timerstate.Store) (appStages, error) {
	var stages appStages
	if chatStage != nil {
		chatStage.Name = "wschat"
	}
	stages.chat = chatStage

	generationStore := generation.NewStore()
	historyStore := conversationhistory.NewStore()
	agentStatusStore := agentstatus.NewStore()

	var err error
	stages.tts, err = tts.NewStage(tts.Config{
		Provider: cfg.TTSProvider,
		ElevenLabs: tts.ElevenLabsConfig{
			APIKey: cfg.ElevenLabs.APIKey,
			Voice:  cfg.ElevenLabs.VoiceID,
			Model:  cfg.ElevenLabs.Model,
		},
		Voicevox: tts.VoicevoxConfig{
			Endpoint:   cfg.Voicevox.Endpoint,
			SpeakerID:  cfg.Voicevox.SpeakerID,
			SpeedScale: cfg.Voicevox.SpeedScale,
		},
	})
	if err != nil {
		return appStages{}, fmt.Errorf("failed to init tts stage: %w", err)
	}
	if stages.tts != nil {
		stages.tts.Name = "tts"
	}
	stages.utterance = utterancebuffer.NewStage(utterancebuffer.Config{
		Generation: generationStore,
	})
	if stages.utterance != nil {
		stages.utterance.Name = "utterancebuffer"
	}
	stages.interimStop = interimstopper.NewStage(interimstopper.Config{
		Generation: generationStore,
	})
	stages.interimStop.Name = "interimstopper"
	stages.sessionReset = sessionreset.NewStage(sessionreset.Config{
		IdleTimeout: cfg.ConversationIdleTimeout,
		History:     historyStore,
		Generation:  generationStore,
		AgentStatus: agentStatusStore,
	})
	if stages.sessionReset != nil {
		stages.sessionReset.Name = "sessionreset"
	}
	stages.committer = conversationcommitter.NewStage(conversationcommitter.Config{
		History:    historyStore,
		Generation: generationStore,
	})
	if stages.committer != nil {
		stages.committer.Name = "conversationcommitter"
	}
	toolSchemas, toolHandlers, toolModes := buildToolRegistry(cfg, timerStore, generationStore)
	stages.llm, err = llm.NewStage(llm.Config{
		APIKey:       cfg.APIKey,
		Model:        cfg.ResponsesModel,
		Instructions: cfg.SystemPrompt,
		History:      historyStore,
		AgentStatus:  agentStatusStore,
		Timers:       timerStore,
		ToolSchemas:  toolSchemas,
	})
	if err != nil {
		if stages.tts != nil {
			stages.tts.Close()
		}
		return appStages{}, fmt.Errorf("failed to init llm stage: %w", err)
	}
	if stages.llm != nil {
		stages.llm.Name = "llm"
	}
	stages.sessionAct = sessionactivate.NewStage(sessionactivate.Config{AgentStatus: agentStatusStore})
	stages.sessionAct.Name = "sessionactivate"
	stages.filterLLM = generationfilter.NewStage(generationfilter.Config{Generation: generationStore})
	stages.filterLLM.Name = "generationfilter-llm"
	stages.filterTTS = generationfilter.NewStage(generationfilter.Config{Generation: generationStore})
	stages.filterTTS.Name = "generationfilter-tts"
	stages.filterSched = generationfilter.NewStage(generationfilter.Config{Generation: generationStore})
	stages.filterSched.Name = "generationfilter-scheduler"
	stages.scheduler = scheduler.NewStage(scheduler.Config{})
	stages.scheduler.Name = "scheduler"
	stages.router = router.NewStage(router.Config{})
	stages.router.Name = "router"
	stages.tool = toolcaller.NewStage(toolHandlers, toolModes)
	if stages.tool != nil {
		stages.tool.Name = "toolcaller"
	}
	stages.rtcpeer, err = rtcpeer.NewStage(rtcpeer.Config{
		IceHostIPs: cfg.RTCIceHostIPs,
	})
	if err != nil {
		if stages.tts != nil {
			stages.tts.Close()
		}
		return appStages{}, fmt.Errorf("failed to init rtcpeer stage: %w", err)
	}
	if stages.rtcpeer != nil {
		stages.rtcpeer.Name = "rtcpeer"
	}
	stages.rtcvad, err = rtcvad.NewStage(rtcvad.Config{
		Generation: generationStore,
	})
	if err != nil {
		if stages.tts != nil {
			stages.tts.Close()
		}
		return appStages{}, fmt.Errorf("failed to init rtcvad stage: %w", err)
	}
	if stages.rtcvad != nil {
		stages.rtcvad.Name = "rtcvad"
	}
	stages.stt, err = buildSTTStage(cfg)
	if err != nil {
		if stages.tts != nil {
			stages.tts.Close()
		}
		return appStages{}, fmt.Errorf("failed to init stt stage: %w", err)
	}
	if stages.stt != nil {
		stages.stt.Name = "stt"
	}
	stages.rtcout, err = rtcout.NewStage(rtcout.Config{})
	if err != nil {
		if stages.tts != nil {
			stages.tts.Close()
		}
		return appStages{}, fmt.Errorf("failed to init rtcout stage: %w", err)
	}
	if stages.rtcout != nil {
		stages.rtcout.Name = "rtcout"
	}
	return stages, nil
}

func buildSTTStage(cfg app.Config) (*graph.Stage, error) {
	provider := strings.TrimSpace(cfg.STTProvider)
	if provider == "" {
		provider = "google"
	}
	switch provider {
	case "google":
		return stt.NewStage(stt.Config{
			SpeechProjectID:  cfg.GoogleCloudProject,
			SpeechRecognizer: cfg.GoogleRecognizer,
			SpeechLanguage:   cfg.GoogleLanguage,
			SpeechCredsJSON:  cfg.GoogleCredentials,
			SpeechPhrases:    cfg.STTPhrases,
		})
	case "openai":
		return openaistt.NewStage(openaistt.Config{
			APIKey:  cfg.APIKey,
			Model:   cfg.OpenAISTTModel,
			Phrases: cfg.STTPhrases,
		})
	default:
		return nil, fmt.Errorf("stt: unknown provider %q", provider)
	}
}

func buildToolRegistry(cfg app.Config, timerStore *timerstate.Store, generationStore *generation.Store) ([]any, map[string]tools.Handler, map[string]string) {
	switchBotClient := buildSwitchBotClient(cfg.SwitchBot)
	var scenes []switchbot.Scene
	if switchBotClient != nil {
		scenes = loadSwitchBotScenes(switchBotClient)
	}
	timerTool := timerfunc.New(timerfunc.Config{
		Store:      timerStore,
		Generation: generationStore,
	})

	reg := registry.New(registry.Config{
		SwitchBotClient: switchBotClient,
		SwitchBotScenes: scenes,
		OpenAIAPIKey:    cfg.APIKey,
		OpenAIModel:     cfg.ResponsesModel,
		TimerTool:       timerTool,
	})
	return reg.DefinitionsForLLM(), reg.Handlers(), reg.ToolModes()
}

func buildSwitchBotClient(cfg app.SwitchBotConfig) *switchbot.Client {
	if strings.TrimSpace(cfg.Token) == "" || strings.TrimSpace(cfg.Secret) == "" {
		log.Println("switchbot: token or secret not set; switchbot tools disabled")
		return nil
	}
	return switchbot.NewSwitchbotClient(cfg.Token, cfg.Secret, cfg.DeviceMap)
}

func loadSwitchBotScenes(client *switchbot.Client) []switchbot.Scene {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	scenes, err := client.ListScenes(ctx)
	if err != nil {
		log.Printf("switchbot: failed to list scenes; switchbot_execute_scene disabled: %v", err)
		return nil
	}
	return scenes
}

func buildHTTPServer(cfg app.Config, timerStore *timerstate.Store) (*http.Server, *graph.Stage, error) {
	mux := http.NewServeMux()
	registerWebUI(mux, cfg.WebDistDir)
	oauthgooglecalendar.RegisterHTTPHandlers(mux)
	server := &http.Server{
		Addr:    cfg.WSAddr,
		Handler: mux,
	}
	chat := wschat.NewStage(mux, wschat.Config{TimerStore: timerStore})
	return server, chat, nil
}

func runHTTPServer(server *http.Server) {
	if server == nil {
		return
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http server listen error: %v", err)
		}
	}()
}

func wireGraph(g *graph.Graph, stages appStages) {
	add := func(stage *graph.Stage) *graph.Node {
		if stage == nil {
			return nil
		}
		return g.AddNode(stage)
	}

	chatNode := add(stages.chat)
	rtcpeerNode := add(stages.rtcpeer)
	rtcvadNode := add(stages.rtcvad)
	sttNode := add(stages.stt)
	interimStopNode := add(stages.interimStop)
	rtcoutNode := add(stages.rtcout)
	utteranceNode := add(stages.utterance)
	sessionResetNode := add(stages.sessionReset)
	committerNode := add(stages.committer)
	llmNode := add(stages.llm)
	sessionActNode := add(stages.sessionAct)
	filterLLMNode := add(stages.filterLLM)
	ttsNode := add(stages.tts)
	filterTTSNode := add(stages.filterTTS)
	schedulerNode := add(stages.scheduler)
	filterSchedNode := add(stages.filterSched)
	routerNode := add(stages.router)
	toolNode := add(stages.tool)

	connectKinds(g, chatNode, rtcpeerNode, types.EventRTCSignal)
	connectKinds(g, rtcpeerNode, chatNode, types.EventRTCSignal)
	connectKinds(g, rtcpeerNode, rtcvadNode, types.EventRTCPeerAudioFrame)
	connectKinds(g, rtcpeerNode, rtcoutNode, types.EventRTCPeerOutputSink)
	connectKinds(g, rtcvadNode, chatNode, types.EventSpeechStart, types.EventSpeechEnd, types.EventRTCVADStatus)
	connectKinds(g, rtcvadNode, sttNode, types.EventRTCSpeechAudio)
	connectKinds(g, sttNode, interimStopNode, types.EventHumanInterimUtterance, types.EventHumanUtterance)
	connectKinds(g, interimStopNode, utteranceNode, types.EventHumanUtterance)
	connectKinds(g, utteranceNode, committerNode, types.EventConversationCommitRequest)
	connectKinds(g, utteranceNode, sessionResetNode, types.EventConversationCommitRequest)
	connectKinds(g, sessionResetNode, chatNode, types.EventSessionReset)
	connectKinds(g, committerNode, llmNode, types.EventLLMRequest)
	connectKinds(g, committerNode, chatNode, types.EventRealtimeOutput)
	connectKinds(g, llmNode, sessionActNode, types.EventTimelineItem, types.EventAgentTimelineEnd)
	connectKinds(g, sessionActNode, filterLLMNode, types.EventTimelineItem, types.EventAgentTimelineEnd)
	connectKinds(g, filterLLMNode, ttsNode, types.EventTimelineItem, types.EventAgentTimelineEnd)
	connectKinds(g, ttsNode, filterTTSNode, types.EventTimelineItem, types.EventPlayableSpeech, types.EventAgentTimelineEnd)
	connectKinds(g, filterTTSNode, schedulerNode, types.EventTimelineItem, types.EventPlayableSpeech, types.EventAgentTimelineEnd)
	connectKinds(g, schedulerNode, filterSchedNode, types.EventScheduledItem, types.EventAgentSpeechPlaybackEnd)
	connectKinds(g, filterSchedNode, chatNode, types.EventAgentSpeechPlaybackEnd)
	connectKinds(g, filterSchedNode, routerNode, types.EventScheduledItem)
	connectKinds(g, routerNode, rtcoutNode, types.EventRealtimeAudio)
	connectKinds(g, routerNode, committerNode, types.EventConversationCommitRequest)
	connectKinds(g, routerNode, toolNode, types.EventToolRequest)
	connectKinds(g, toolNode, committerNode, types.EventConversationCommitRequest)
	connectKinds(g, toolNode, chatNode, types.EventWhiteboardUpdate, types.EventTimerState)
}

func connectKinds(g *graph.Graph, from, to *graph.Node, kinds ...types.EventKind) {
	if from == nil || to == nil {
		return
	}
	g.ConnectKinds(from, to, kinds...)
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
