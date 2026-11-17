package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/tetetratra/smart-speaker/internal/app"
	"github.com/tetetratra/smart-speaker/internal/automations"
	"github.com/tetetratra/smart-speaker/internal/automations/script"
	"github.com/tetetratra/smart-speaker/internal/components/audioframe"
	"github.com/tetetratra/smart-speaker/internal/components/audiopace"
	"github.com/tetetratra/smart-speaker/internal/components/automation"
	"github.com/tetetratra/smart-speaker/internal/components/conversationmanager"
	"github.com/tetetratra/smart-speaker/internal/components/gateway"
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
	"github.com/tetetratra/smart-speaker/internal/plugin"
	"github.com/tetetratra/smart-speaker/internal/plugins/claudecode"
	"github.com/tetetratra/smart-speaker/internal/plugins/googlecalendar"
	mcpplugin "github.com/tetetratra/smart-speaker/internal/plugins/mcp"
	"github.com/tetetratra/smart-speaker/internal/plugins/playbackvolume"
	"github.com/tetetratra/smart-speaker/internal/plugins/switchbot"
	"github.com/tetetratra/smart-speaker/internal/plugins/userlocation"
	"github.com/tetetratra/smart-speaker/internal/plugins/websearch"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/tools/execution"
	"github.com/tetetratra/smart-speaker/internal/tools/registry"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func main() {
	log.SetFlags(0)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	setLocalTimeZone()

	cfg := app.LoadConfig()
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
	services := runtimeServices{AppContext: ctx, TTS: ttsSelector, History: historyStore}
	// グラフの外からの入力（Claude Codeの通知・Web検索の結果・位置の変化）は、gateway を通ってグラフへ入る。
	// サービスはグラフのコンポーネントより先に作るので、gateway だけ先に作って送り口を渡す。
	services.Gateway = gateway.New()
	services.Deliver = services.Gateway.Deliver
	services.Plugins, err = buildPlugins(plugin.Deps{AppContext: ctx, Deliver: services.Deliver, Config: cfg.DecodePluginConfig, DataDir: filepath.Dir(cfg.Memory.StorePath),
		OpenAI: plugin.OpenAIConfig{APIKey: cfg.APIKey, ResponsesModel: cfg.ResponsesModel}})
	if err != nil {
		log.Fatal(err)
	}
	if unknown := plugin.UnknownNames(services.Plugins, cfg.PluginConfigNames()); len(unknown) > 0 {
		log.Fatalf("config: config.yaml の plugins に、組み込まれていないプラグインの名前があります: %s", strings.Join(unknown, ", "))
	}
	services.Automations, err = automations.New(cfg.AutomationStorePath, func(ctx context.Context, delivery automations.Delivery) error {
		return services.Runner.RunWithEvent(ctx, delivery.ID, delivery.Name, delivery.Script, delivery.Event)
	})
	if err != nil {
		log.Fatal(err)
	}
	services.Automations.SetSources(plugin.Sources(services.Plugins))
	// 予約ジョブはグラフのコンポーネントで、実行結果は gateway を通さず会話管理へ送る。
	// Runner はコンポーネントより先に作るので、コンポーネントだけ先に作って送り口を渡す。
	services.AutomationStage = automation.New(services.Automations)
	services.WakeMode, err = wakemode.NewStore(cfg.WakeModeStorePath, cfg.WakeSettings)
	if err != nil {
		log.Fatal(err)
	}
	services.ToolSchemas, services.ToolHandlers = buildToolRegistry(cfg, memoryStore, services)
	executor := execution.New(services.ToolHandlers)
	services.Runner = script.New(executor.Execute, services.AutomationStage.Deliver).WithTitle(executor.Title)
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

func closeStages(stages ...*graph.Stage) {
	for _, st := range stages {
		if st != nil {
			st.Close()
		}
	}
}

type appStages struct {
	chat, rtcpeer, realtime, manager, segment, tts, frame, pace, tool, wakegate, output, gateway, automation *graph.Stage
}

func (s appStages) all() []*graph.Stage {
	return []*graph.Stage{s.chat, s.rtcpeer, s.realtime, s.manager, s.segment, s.tts, s.frame, s.pace, s.tool, s.wakegate, s.output, s.gateway, s.automation}
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
	gw := services.Gateway
	if gw == nil {
		gw = gateway.New()
	}
	memoryCreatorHook, err := memoryhook.NewCreatorHook(memoryhook.CreatorHookConfig{
		OperationCreator: memoryOperationCreator,
		Memory:           memoryStore,
		Deliver:          gw.Deliver,
		MemorizeAfter:    cfg.Memory.MemorizeAfter,
		MaxHistoryBytes:  cfg.Memory.HistoryMaxBytes,
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
		Instructions:  plugin.Instructions(cfg.SystemPrompt, services.Plugins), Generation: generationStore, Tools: toolSchemas,
		ToolTitle:   execution.New(toolHandlers).Title,
		IdleTimeout: cfg.ConversationIdleTimeout,
	})
	if err != nil {
		return appStages{}, err
	}
	stages.gateway = gw.Stage()
	if services.AutomationStage != nil {
		stages.automation = services.AutomationStage.Stage()
	}
	stages.manager = conversationmanager.NewStage(conversationmanager.Config{History: historyStore,
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
	reg := registry.New(registry.Config{AppContext: services.AppContext,
		WakeMode:    services.WakeMode,
		Automations: services.Automations,
		Memory:      memoryStore,
		PluginTools: plugin.Tools(services.Plugins),
	})
	return reg.DefinitionsForRealtime(), reg.Handlers()
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
	gw := add(s.gateway)
	connect(g, gw, manager, types.EventConversationInput{})
	automation := add(s.automation)
	connect(g, manager, automation, types.EventConversationInput{})
	connect(g, automation, manager, types.EventConversationInput{})
	connect(g, manager, chat, types.EventConversationRecordAdded{})
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

// buildPlugins は本体に組み込むプラグインを作る。並び順はツールの並び順になる。
func buildPlugins(deps plugin.Deps) ([]plugin.Plugin, error) {
	switchBot, err := switchbot.New(deps)
	if err != nil {
		return nil, err
	}
	calendar, err := googlecalendar.New(deps)
	if err != nil {
		return nil, err
	}
	claudeCode, err := claudecode.New(deps)
	if err != nil {
		return nil, err
	}
	mcp, err := mcpplugin.New(deps)
	if err != nil {
		return nil, err
	}
	return []plugin.Plugin{
		userlocation.New(deps),
		playbackvolume.New(deps),
		switchBot,
		websearch.New(deps),
		calendar,
		claudeCode,
		mcp,
	}, nil
}

// connect は from から to へ、events と同じ型のイベントだけを流す。events にはゼロ値を渡す。
func connect(g *graph.Graph, from, to *graph.Node, events ...types.Event) {
	if from == nil || to == nil {
		return
	}
	g.Connect(from, to, events...)
}
