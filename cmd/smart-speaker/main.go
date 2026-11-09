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
	"github.com/tetetratra/smart-speaker/internal/components/audioframe"
	"github.com/tetetratra/smart-speaker/internal/components/audiopace"
	"github.com/tetetratra/smart-speaker/internal/components/generationfilter"
	"github.com/tetetratra/smart-speaker/internal/components/realtime"
	"github.com/tetetratra/smart-speaker/internal/components/rtcout"
	"github.com/tetetratra/smart-speaker/internal/components/rtcpeer"
	"github.com/tetetratra/smart-speaker/internal/components/speechsegment"
	"github.com/tetetratra/smart-speaker/internal/components/toolcaller"
	"github.com/tetetratra/smart-speaker/internal/components/tts"
	"github.com/tetetratra/smart-speaker/internal/components/wakegate"
	"github.com/tetetratra/smart-speaker/internal/conversation"
	"github.com/tetetratra/smart-speaker/internal/graph"
	memoryhook "github.com/tetetratra/smart-speaker/internal/hooks/memory"
	oauthgooglecalendar "github.com/tetetratra/smart-speaker/internal/oauth/googlecalendar"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"github.com/tetetratra/smart-speaker/internal/tools"
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
	services := runtimeServices{TTS: ttsSelector, ToolHistory: toolhistory.NewStore(200)}
	scheduled := make(chan types.ScheduledPrompt, 100)
	services.Scheduled = scheduled
	direct := make(chan types.DirectToolCall, 100)
	services.DirectToolCalls = direct
	services.AutomationRunner = &automationRunner{scheduled: scheduled, direct: direct, history: services.ToolHistory}
	services.Automations, err = automations.New(cfg.AutomationStorePath, services.AutomationRunner.run)
	if err != nil {
		log.Fatal(err)
	}
	services.ClaudeCode = newClaudeCodeService(cfg.AgentBridge, scheduled)
	services.WakeMode, err = wakemode.NewStore(cfg.WakeModeStorePath, cfg.WakeSettings)
	if err != nil {
		log.Fatal(err)
	}
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
	chat, rtcpeer, realtime, segment, tts, frame, pace, tool, wakegate, output *graph.Stage
}

func (s appStages) all() []*graph.Stage {
	return []*graph.Stage{s.chat, s.rtcpeer, s.realtime, s.segment, s.tts, s.frame, s.pace, s.tool, s.wakegate, s.output}
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
	historyStore := conversationhistory.NewStore()
	if memoryStore == nil {
		return appStages{}, fmt.Errorf("memory store is required")
	}
	memoryContextProvider, err := memoryhook.NewContextProvider(memoryhook.ContextProviderConfig{
		Memory: memoryStore,
	})
	if err != nil {
		return appStages{}, fmt.Errorf("failed to init memory context provider: %w", err)
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

	toolSchemas, toolHandlers, toolModes, responsePolicies := buildToolRegistry(cfg, memoryStore, services)
	if services.AutomationRunner != nil {
		services.AutomationRunner.handlers = toolHandlers
	}
	stages.realtime, err = realtime.NewStage(realtime.Config{
		ToolHistory:     services.ToolHistory,
		WakeMode:        services.WakeMode,
		Scheduled:       services.Scheduled,
		DirectToolCalls: services.DirectToolCalls,
		APIKey:          cfg.APIKey, Model: cfg.RealtimeModel, Endpoint: cfg.RealtimeEndpoint, TranscriptionModel: cfg.RealtimeTranscriptionModel,
		TurnDetection: cfg.RealtimeVAD,
		Instructions:  cfg.SystemPrompt, Generation: generationStore, History: historyStore, Tools: toolSchemas, ToolResponsePolicies: responsePolicies,
		IdleTimeout: cfg.ConversationIdleTimeout,
		Context:     conversation.ContextBuilder{APIKey: cfg.APIKey, Model: cfg.ResponsesModel},
		MemoryContext: func(ctx context.Context) (string, error) {
			messages, err := memoryContextProvider.BuildContext(ctx, nil)
			var parts []string
			for _, m := range messages {
				parts = append(parts, m.Content)
			}
			return strings.Join(parts, "\n"), err
		},
		OnConversationEnd: memoryCreatorHook.ExecRecords,
	})
	if err != nil {
		return appStages{}, err
	}
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
	stages.tool = toolcaller.NewStage(toolHandlers, toolModes, services.ToolHistory)
	for _, stage := range []*graph.Stage{stages.segment, stages.tts, stages.frame, stages.pace, stages.chat, stages.output} {
		if stage != nil {
			generationfilter.Attach(stage, generationStore)
		}
	}
	generationfilter.AttachToolRequests(stages.tool, generationStore)
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

func buildToolRegistry(cfg app.Config, memoryStore *memorystate.Store, services runtimeServices) ([]any, map[string]tools.Handler, map[string]string, map[string]tools.ResponsePolicy) {
	switchBotClient := buildSwitchBotClient(cfg.SwitchBot)
	var scenes []switchbot.Scene
	if switchBotClient != nil {
		scenes = loadSwitchBotScenes(switchBotClient)
	}

	reg := registry.New(registry.Config{
		WakeMode:        services.WakeMode,
		Automations:     services.Automations,
		Memory:          memoryStore,
		ClaudeCode:      services.ClaudeCode,
		SwitchBotClient: switchBotClient,
		SwitchBotScenes: scenes,
		OpenAIAPIKey:    cfg.APIKey,
		OpenAIModel:     cfg.ResponsesModel,
	})
	return reg.DefinitionsForRealtime(), reg.Handlers(), reg.ToolModes(), reg.ResponsePolicies()
}

// newClaudeCodeService はブリッジの設定があるときだけ、Claude Code連携を有効にする。
func newClaudeCodeService(cfg app.AgentBridgeConfig, scheduled chan<- types.ScheduledPrompt) *agentbridge.Service {
	if cfg.URL == "" || cfg.Token == "" {
		log.Println("agent-bridge: agent_bridge.url in config.yaml or AGENT_BRIDGE_TOKEN not set; claude code tools disabled")
		return nil
	}
	return agentbridge.NewService(agentbridge.NewClient(cfg.URL, cfg.Token), func(ctx context.Context, prompt types.ScheduledPrompt) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case scheduled <- prompt:
			return nil
		}
	})
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
	output := add(s.output)
	connectKinds(g, peer, output, types.EventRTCPeerOutputSink)
	connectKinds(g, chat, output, types.EventRTCSignal)
	segment, ttsNode, frame, pace, tool := add(s.segment), add(s.tts), add(s.frame), add(s.pace), add(s.tool)
	connectKinds(g, chat, peer, types.EventRTCSignal)
	connectKinds(g, peer, chat, types.EventRTCSignal)
	wake := add(s.wakegate)
	connectKinds(g, peer, wake, types.EventRTCPeerAudioFrame, types.EventRTCPeerOutputSink)
	connectKinds(g, wake, rt, types.EventRTCPeerAudioFrame, types.EventRTCPeerOutputSink, types.EventLocalWakeUtterance)
	connectKinds(g, wake, chat, types.EventWakeTranscription)
	connectKinds(g, rt, segment, types.EventResponseText)
	connectKinds(g, segment, ttsNode, types.EventTimelineItem, types.EventAgentTimelineEnd)
	connectKinds(g, ttsNode, frame, types.EventPlayableSpeech, types.EventAgentTimelineEnd)
	connectKinds(g, frame, pace, types.EventAudioFrame)
	connectKinds(g, pace, output, types.EventAudioFrame)
	connectKinds(g, output, chat, types.EventAudioSent)
	connectKinds(g, output, rt, types.EventAudioSent)
	connectKinds(g, rt, chat, types.EventGenerationChanged, types.EventRealtimeOutput, types.EventSpeechStart, types.EventSpeechEnd, types.EventRealtimeStatus, types.EventSessionReset)
	connectKinds(g, rt, tool, types.EventToolRequest)
	connectKinds(g, tool, rt, types.EventConversationCommitRequest)
	connectKinds(g, tool, chat, types.EventWhiteboardUpdate)
}

func connectKinds(g *graph.Graph, from, to *graph.Node, kinds ...types.EventKind) {
	if from == nil || to == nil {
		return
	}
	g.ConnectKinds(from, to, kinds...)
}
