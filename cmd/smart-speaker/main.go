package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tetetratra/smart-speaker/internal/agentbridge"
	"github.com/tetetratra/smart-speaker/internal/app"
	"github.com/tetetratra/smart-speaker/internal/automations"
	"github.com/tetetratra/smart-speaker/internal/automations/script"
	"github.com/tetetratra/smart-speaker/internal/components/audioframe"
	"github.com/tetetratra/smart-speaker/internal/components/audiopace"
	"github.com/tetetratra/smart-speaker/internal/components/conversationmanager"
	"github.com/tetetratra/smart-speaker/internal/components/generationfilter"
	"github.com/tetetratra/smart-speaker/internal/components/realtime"
	"github.com/tetetratra/smart-speaker/internal/components/rtcout"
	"github.com/tetetratra/smart-speaker/internal/components/rtcpeer"
	"github.com/tetetratra/smart-speaker/internal/components/speechsegment"
	"github.com/tetetratra/smart-speaker/internal/components/toolcaller"
	"github.com/tetetratra/smart-speaker/internal/components/tts"
	"github.com/tetetratra/smart-speaker/internal/components/wakegate"
	"github.com/tetetratra/smart-speaker/internal/graph"
	memoryhook "github.com/tetetratra/smart-speaker/internal/hooks/memory"
	oauthgooglecalendar "github.com/tetetratra/smart-speaker/internal/oauth/googlecalendar"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	"github.com/tetetratra/smart-speaker/internal/states/playbackvolume"
	"github.com/tetetratra/smart-speaker/internal/states/userlocation"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/tools/execution"
	"github.com/tetetratra/smart-speaker/internal/tools/functions/switchbot"
	"github.com/tetetratra/smart-speaker/internal/tools/registry"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func main() {
	log.SetFlags(0)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	setLocalTimeZone()

	cfg := app.LoadConfig()
	oauthgooglecalendar.Configure(cfg.GoogleCalendar)

	ensureGoogleCalendarToken()

	memoryStore, err := memorystate.NewStore(cfg.Memory.StorePath)
	if err != nil {
		log.Fatalf("failed to init memory store: %v", err)
	}

	ttsSelector, err := tts.NewSelector(ttsConfig(cfg))
	if err != nil {
		log.Fatal(err)
	}
	historyStore, err := conversationhistory.NewStore(cfg.ConversationHistoryPath)
	if err != nil {
		log.Fatalf("failed to init conversation history: %v", err)
	}
	services := runtimeServices{UserLocation: &userlocation.Store{}, PlaybackVolume: &playbackvolume.Store{}, AppContext: ctx, TTS: ttsSelector, History: historyStore}
	inbound := make(chan types.InboundMessage, 100)
	services.Inbound = inbound
	// 会話の外からの差し込み（予約ジョブ・Claude Codeの通知・Web検索の結果）を、会話管理へ送る。応答依頼か情報追加かは会話管理の入口で変換する。
	services.Deliver = func(ctx context.Context, prompt types.InboundMessage) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case inbound <- prompt:
			return nil
		}
	}
	services.Automations, err = automations.New(cfg.AutomationStorePath, func(ctx context.Context, delivery automations.Delivery) error {
		return services.Runner.Run(ctx, delivery.ID, delivery.Name, delivery.Script)
	})
	if err != nil {
		log.Fatal(err)
	}
	services.ClaudeCode = newClaudeCodeService(cfg.AgentBridge, services.Deliver)
	services.WakeMode, err = wakemode.NewStore(cfg.WakeModeStorePath, cfg.WakeSettings)
	if err != nil {
		log.Fatal(err)
	}
	services.ToolSchemas, services.ToolHandlers = buildToolRegistry(cfg, memoryStore, services)
	executor := execution.New(services.ToolHandlers)
	services.Runner = script.New(executor.Execute, services.Deliver).WithTitle(executor.Title)
	server, chatStage, err := buildHTTPServer(cfg, memoryStore, services)
	if err != nil {
		log.Fatal(err)
	}
	defer closeHTTPServer(server)

	stages, err := buildStages(cfg, chatStage, memoryStore, services)
	if err != nil {
		log.Fatal(err)
	}
	defer closeStages(stages.all()...)

	g := graph.New()
	defer g.Close()

	wireGraph(g, stages)
	runHTTPServer(server)
	schedulerDone := make(chan struct{})
	go func() { defer close(schedulerDone); services.Automations.Run(ctx) }()
	defer func() { cancel(); <-schedulerDone }()

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

func closeStages(stages ...*graph.Stage) {
	for _, st := range stages {
		if st != nil {
			st.Close()
		}
	}
}

type appStages struct {
	chat, rtcpeer, realtime, manager, segment, tts, frame, pace, tool, wakegate, output *graph.Stage
}

func (s appStages) all() []*graph.Stage {
	return []*graph.Stage{s.chat, s.rtcpeer, s.realtime, s.manager, s.segment, s.tts, s.frame, s.pace, s.tool, s.wakegate, s.output}
}

func ttsConfig(cfg app.Config) tts.Config {
	return tts.Config{
		Provider:   cfg.TTSProvider,
		ElevenLabs: tts.ElevenLabsConfig{APIKey: cfg.ElevenLabs.APIKey, Voice: cfg.ElevenLabs.VoiceID, Model: cfg.ElevenLabs.Model},
		Voicevox:   tts.VoicevoxConfig{Endpoint: cfg.Voicevox.Endpoint, SpeakerID: cfg.Voicevox.SpeakerID, SpeedScale: cfg.Voicevox.SpeedScale},
	}
}

func buildStages(cfg app.Config, chatStage *graph.Stage, memoryStore *memorystate.Store, services runtimeServices) (appStages, error) {
	var stages appStages
	stages.chat = chatStage
	if chatStage != nil {
		chatStage.Name = "wschat"
	}
	generationStore := generation.NewStore()
	historyStore := services.History
	if historyStore == nil {
		var err error
		if historyStore, err = conversationhistory.NewStore(cfg.ConversationHistoryPath); err != nil {
			return appStages{}, err
		}
	}
	if memoryStore == nil {
		return appStages{}, fmt.Errorf("memory store is required")
	}
	memoryOperationCreator, err := memoryhook.NewOpenAIClient(memoryhook.OpenAIClientConfig{
		APIKey: cfg.APIKey,
		Model:  cfg.Memory.Model,
	})
	if err != nil {
		return appStages{}, fmt.Errorf("failed to init memory operation creator: %w", err)
	}
	memoryCreatorHook, err := memoryhook.NewCreatorHook(memoryhook.CreatorHookConfig{
		OperationCreator: memoryOperationCreator,
		Memory:           memoryStore,
	})
	if err != nil {
		return appStages{}, fmt.Errorf("failed to init memory creator hook: %w", err)
	}

	toolSchemas, toolHandlers := services.ToolSchemas, services.ToolHandlers
	if toolHandlers == nil {
		toolSchemas, toolHandlers = buildToolRegistry(cfg, memoryStore, services)
	}
	stages.realtime, err = realtime.NewStage(realtime.Config{History: historyStore, Memory: memoryStore,
		WakeMode: services.WakeMode,
		APIKey:   cfg.APIKey, Model: cfg.RealtimeModel, Endpoint: cfg.RealtimeEndpoint, TranscriptionModel: cfg.RealtimeTranscriptionModel,
		TurnDetection: cfg.RealtimeVAD,
		Instructions:  cfg.SystemPrompt, Generation: generationStore, Tools: toolSchemas,
		ToolTitle:   execution.New(toolHandlers).Title,
		IdleTimeout: cfg.ConversationIdleTimeout,
	})
	if err != nil {
		return appStages{}, err
	}
	stages.manager = conversationmanager.NewStage(conversationmanager.Config{History: historyStore, Inbound: services.Inbound,
		OnIdle: func(ctx context.Context, keep map[string]bool) {
			memoryCreatorHook.MemorizeOld(ctx, historyStore, keep)
		},
	})

	ttsCfg := ttsConfig(cfg)
	ttsCfg.Selector = services.TTS
	stages.tts, err = tts.NewStage(ttsCfg)
	if err != nil {
		return appStages{}, err
	}
	stages.segment = speechsegment.NewStage()
	stages.frame = audioframe.NewStage()
	stages.pace = audiopace.NewStage()
	stages.output, err = rtcout.NewStage(rtcout.Config{})
	if err != nil {
		return appStages{}, err
	}
	stages.tool = toolcaller.NewStage(toolHandlers)
	for _, stage := range []*graph.Stage{stages.segment, stages.tts, stages.frame, stages.pace, stages.chat, stages.output} {
		if stage != nil {
			generationfilter.Attach(stage, generationStore)
		}
	}
	stages.wakegate = wakegate.NewStage(wakegate.Config{Store: services.WakeMode, STT: &wakegate.HTTPClient{Endpoint: cfg.LocalSTTEndpoint}, EnergyThreshold: cfg.WakeEnergyThreshold})
	stages.rtcpeer, err = rtcpeer.NewStage(rtcpeer.Config{IceAdvertiseIPs: cfg.RTCIceAdvertiseIPs})
	if err != nil {
		closeStages(stages.all()...)
		return appStages{}, err
	}
	stages.tts.Name = "tts"
	stages.tool.Name = "toolcaller"
	stages.rtcpeer.Name = "rtcpeer"
	return stages, nil
}

func buildToolRegistry(cfg app.Config, memoryStore *memorystate.Store, services runtimeServices) ([]any, map[string]tools.Handler) {
	switchBotClient := buildSwitchBotClient(cfg.SwitchBot)
	var scenes []switchbot.Scene
	if switchBotClient != nil {
		scenes = loadSwitchBotScenes(switchBotClient)
	}

	reg := registry.New(registry.Config{AppContext: services.AppContext,
		WakeMode:         services.WakeMode,
		UserLocation:     services.UserLocation,
		PlaybackVolume:   services.PlaybackVolume,
		Automations:      services.Automations,
		Memory:           memoryStore,
		ClaudeCode:       services.ClaudeCode,
		SwitchBotClient:  switchBotClient,
		SwitchBotScenes:  scenes,
		OpenAIAPIKey:     cfg.APIKey,
		OpenAIModel:      cfg.ResponsesModel,
		WebSearchDeliver: services.Deliver,
	})
	return reg.DefinitionsForRealtime(), reg.Handlers()
}

// newClaudeCodeService はブリッジの設定があるときだけ、Claude Code連携を有効にする。
func newClaudeCodeService(cfg app.AgentBridgeConfig, deliver func(context.Context, types.InboundMessage) error) *agentbridge.Service {
	if cfg.URL == "" || cfg.Token == "" {
		log.Println("agent-bridge: agent_bridge.url in config.yaml or AGENT_BRIDGE_TOKEN not set; claude code tools disabled")
		return nil
	}
	return agentbridge.NewService(agentbridge.NewClient(cfg.URL, cfg.Token), deliver)
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

func wireGraph(g *graph.Graph, s appStages) {
	add := func(st *graph.Stage) *graph.Node {
		if st == nil {
			return nil
		}
		return g.AddNode(st)
	}
	chat, peer, rt := add(s.chat), add(s.rtcpeer), add(s.realtime)
	manager := add(s.manager)
	connect(g, rt, manager, types.EventConversationInput{}, types.EventConversationHistorySync{}, types.EventConversationIdle{}, types.EventConversationRecordsUsed{})
	connect(g, manager, rt, types.EventConversationInput{}, types.EventConversationHistorySync{})
	connect(g, manager, chat, types.EventConversationRecordChanged{})
	output := add(s.output)
	connect(g, peer, output, types.EventRTCPeerOutputSink{})
	segment, ttsNode, frame, pace, tool := add(s.segment), add(s.tts), add(s.frame), add(s.pace), add(s.tool)
	connect(g, chat, peer, types.EventRTCSignal{})
	connect(g, peer, chat, types.EventRTCSignal{})
	wake := add(s.wakegate)
	connect(g, peer, wake, types.EventRTCPeerAudioFrame{}, types.EventRTCPeerOutputSink{})
	connect(g, wake, rt, types.EventRTCPeerAudioFrame{}, types.EventRTCPeerOutputSink{}, types.EventLocalWakeUtterance{})
	connect(g, wake, chat, types.EventWakeTranscription{})
	connect(g, rt, segment, types.EventResponseText{})
	connect(g, segment, ttsNode, types.EventTimelineItem{}, types.EventAgentTimelineEnd{})
	connect(g, ttsNode, frame, types.EventPlayableSpeech{}, types.EventAgentTimelineEnd{})
	connect(g, frame, pace, types.EventAudioFrame{})
	connect(g, pace, output, types.EventAudioFrame{})
	connect(g, output, chat, types.EventAudioSent{})
	connect(g, output, rt, types.EventAudioSent{})
	connect(g, rt, chat, types.EventGenerationChanged{}, types.EventRealtimeOutput{}, types.EventSpeechStart{}, types.EventSpeechEnd{}, types.EventRealtimeStatus{})
	connect(g, rt, tool, types.EventToolRequest{})
	connect(g, tool, rt, types.EventConversationCommitRequest{})
}

// connect は from から to へ、events と同じ型のイベントだけを流す。events にはゼロ値を渡す。
func connect(g *graph.Graph, from, to *graph.Node, events ...types.Event) {
	if from == nil || to == nil {
		return
	}
	g.Connect(from, to, events...)
}
