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

	"smart-speaker/internal/app"
	"smart-speaker/internal/components/conversationcommitter"
	"smart-speaker/internal/components/generationfilter"
	"smart-speaker/internal/components/llm"
	"smart-speaker/internal/components/router"
	"smart-speaker/internal/components/rtc"
	"smart-speaker/internal/components/scheduler"
	"smart-speaker/internal/components/toolcaller"
	"smart-speaker/internal/components/tts"
	"smart-speaker/internal/components/utterancebuffer"
	"smart-speaker/internal/components/wschat"
	"smart-speaker/internal/graph"
	oauthgooglecalendar "smart-speaker/internal/oauth/googlecalendar"
	"smart-speaker/internal/states/conversationhistory"
	"smart-speaker/internal/states/generation"
	types "smart-speaker/internal/types"
)

func main() {
	log.SetFlags(0)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	setLocalTimeZone()

	cfg := app.LoadConfig("system_prompt.txt")

	ensureGoogleCalendarToken()

	server, chatStage, err := buildHTTPServer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer closeHTTPServer(server)

	stages, err := buildStages(cfg, chatStage)
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
	chat        *graph.Stage
	rtc         *graph.Stage
	utterance   *graph.Stage
	committer   *graph.Stage
	llm         *graph.Stage
	filterLLM   *graph.Stage
	tts         *graph.Stage
	filterTTS   *graph.Stage
	scheduler   *graph.Stage
	filterSched *graph.Stage
	router      *graph.Stage
	tool        *graph.Stage
}

func (s appStages) all() []*graph.Stage {
	return []*graph.Stage{
		s.chat,
		s.rtc,
		s.utterance,
		s.committer,
		s.llm,
		s.filterLLM,
		s.tts,
		s.filterTTS,
		s.scheduler,
		s.filterSched,
		s.router,
		s.tool,
	}
}

func buildStages(cfg app.Config, chatStage *graph.Stage) (appStages, error) {
	var stages appStages
	if chatStage != nil {
		chatStage.Name = "wschat"
	}
	stages.chat = chatStage

	generationStore := generation.NewStore()
	historyStore := conversationhistory.NewStore()

	var err error
	stages.tts, err = tts.NewStage(tts.Config{
		APIKey: cfg.ElevenLabs.APIKey,
		Voice:  cfg.ElevenLabs.VoiceID,
		Model:  cfg.ElevenLabs.Model,
	})
	if err != nil {
		return appStages{}, fmt.Errorf("failed to init elevenlabs stage: %w", err)
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
	var resultCommitter *conversationcommitter.ResultAPI
	stages.committer, resultCommitter = conversationcommitter.NewStage(conversationcommitter.Config{
		History:    historyStore,
		Generation: generationStore,
	})
	if stages.committer != nil {
		stages.committer.Name = "conversationcommitter"
	}
	stages.llm, err = llm.NewStage(llm.Config{
		APIKey:       cfg.APIKey,
		Model:        cfg.ResponsesModel,
		Instructions: cfg.SystemPrompt,
		History:      historyStore,
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
	stages.tool = toolcaller.NewStage(nil, resultCommitter)
	if stages.tool != nil {
		stages.tool.Name = "toolcaller"
	}
	stages.rtc, err = rtc.NewStage(rtc.Config{
		IceHostIPs:       cfg.RTCIceHostIPs,
		SpeechProjectID:  cfg.GoogleCloudProject,
		SpeechRecognizer: cfg.GoogleRecognizer,
		SpeechLanguage:   cfg.GoogleLanguage,
		SpeechCredsJSON:  cfg.GoogleCredentials,
	})
	if err != nil {
		if stages.tts != nil {
			stages.tts.Close()
		}
		return appStages{}, fmt.Errorf("failed to init rtc stage: %w", err)
	}
	if stages.rtc != nil {
		stages.rtc.Name = "rtc"
	}
	return stages, nil
}

func buildHTTPServer(cfg app.Config) (*http.Server, *graph.Stage, error) {
	mux := http.NewServeMux()
	registerWebUI(mux, cfg.WebDistDir)
	oauthgooglecalendar.RegisterHTTPHandlers(mux)
	server := &http.Server{
		Addr:    cfg.WSAddr,
		Handler: mux,
	}
	chat := wschat.NewStage(mux)
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
	rtcNode := add(stages.rtc)
	utteranceNode := add(stages.utterance)
	committerNode := add(stages.committer)
	llmNode := add(stages.llm)
	filterLLMNode := add(stages.filterLLM)
	ttsNode := add(stages.tts)
	filterTTSNode := add(stages.filterTTS)
	schedulerNode := add(stages.scheduler)
	filterSchedNode := add(stages.filterSched)
	routerNode := add(stages.router)
	toolNode := add(stages.tool)

	connectKinds(g, chatNode, rtcNode, types.EventRTCSignal)
	connectKinds(g, rtcNode, chatNode, types.EventRTCSignal, types.EventSpeechEnd, types.EventRTCVADStatus)
	connectKinds(g, rtcNode, utteranceNode, types.EventHumanUtterance)
	connectKinds(g, utteranceNode, committerNode, types.EventConversationCommitRequest)
	connectKinds(g, committerNode, llmNode, types.EventLLMRequest)
	connectKinds(g, committerNode, chatNode, types.EventRealtimeOutput)
	connectKinds(g, llmNode, filterLLMNode, types.EventTimelineItem)
	connectKinds(g, filterLLMNode, ttsNode, types.EventTimelineItem)
	connectKinds(g, ttsNode, filterTTSNode, types.EventTimelineItem, types.EventPlayableSpeech)
	connectKinds(g, filterTTSNode, schedulerNode, types.EventTimelineItem, types.EventPlayableSpeech)
	connectKinds(g, schedulerNode, filterSchedNode, types.EventScheduledItem)
	connectKinds(g, filterSchedNode, routerNode, types.EventScheduledItem)
	connectKinds(g, routerNode, rtcNode, types.EventRealtimeAudio)
	connectKinds(g, routerNode, committerNode, types.EventConversationCommitRequest)
	connectKinds(g, routerNode, toolNode, types.EventToolRequest)
	connectKinds(g, toolNode, chatNode, types.EventWhiteboardUpdate)
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
