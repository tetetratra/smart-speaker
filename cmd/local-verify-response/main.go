package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/tetetratra/smart-speaker/internal/app"
	"github.com/tetetratra/smart-speaker/internal/components/conversationcommitter"
	"github.com/tetetratra/smart-speaker/internal/components/generationfilter"
	"github.com/tetetratra/smart-speaker/internal/components/llm"
	"github.com/tetetratra/smart-speaker/internal/components/router"
	"github.com/tetetratra/smart-speaker/internal/components/scheduler"
	"github.com/tetetratra/smart-speaker/internal/components/tts"
	"github.com/tetetratra/smart-speaker/internal/components/utterancebuffer"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

const (
	defaultInputText = "こんにちは。短く挨拶だけ返してください。"
	defaultTimeout   = 90 * time.Second
)

type collector struct {
	upstream   chan types.Event
	downstream chan types.Event
	events     chan types.Event
}

func newCollector() (*graph.Stage, <-chan types.Event) {
	c := &collector{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event),
		events:     make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	return &graph.Stage{
		Name:       "collector",
		Upstream:   c.upstream,
		Downstream: c.downstream,
		Run:        c.run,
		CloseFn:    c.close,
	}, c.events
}

func (c *collector) run(ctx context.Context) {
	defer close(c.downstream)
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-c.upstream:
			if !ok {
				return
			}
			select {
			case <-ctx.Done():
				return
			case c.events <- evt:
			}
		}
	}
}

func (c *collector) close() error {
	close(c.upstream)
	return nil
}

func main() {
	log.SetFlags(0)

	inputText := strings.TrimSpace(os.Getenv("LOCAL_VERIFY_TEXT"))
	if inputText == "" {
		inputText = defaultInputText
	}

	cfg := app.LoadConfig("system_prompt.txt")
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	utterance, collected, cleanup := buildPipeline(ctx, cfg)
	defer cleanup()

	utterance.Upstream <- types.Event{
		Kind: types.EventHumanUtterance,
		Payload: types.OutputLine{
			Role:   types.RoleUser,
			Text:   inputText,
			Source: "local-verify",
		},
	}

	if err := waitForResponse(ctx, collected); err != nil {
		log.Fatal(err)
	}
}

func buildPipeline(ctx context.Context, cfg app.Config) (*graph.Stage, <-chan types.Event, func()) {
	generationStore := generation.NewStore()
	historyStore := conversationhistory.NewStore()

	utterance := utterancebuffer.NewStage(utterancebuffer.Config{
		Delay:      20 * time.Millisecond,
		Generation: generationStore,
	})
	utterance.Name = "utterancebuffer"
	committer := conversationcommitter.NewStage(conversationcommitter.Config{
		History:    historyStore,
		Generation: generationStore,
	})
	committer.Name = "conversationcommitter"
	llmStage, err := llm.NewStage(llm.Config{
		APIKey:       cfg.APIKey,
		Model:        cfg.ResponsesModel,
		Instructions: cfg.SystemPrompt,
		History:      historyStore,
	})
	if err != nil {
		log.Fatal(err)
	}
	llmStage.Name = "llm"
	filterLLM := generationfilter.NewStage(generationfilter.Config{Generation: generationStore})
	filterLLM.Name = "generationfilter-llm"
	ttsStage, err := tts.NewStage(tts.Config{
		APIKey: cfg.ElevenLabs.APIKey,
		Voice:  cfg.ElevenLabs.VoiceID,
		Model:  cfg.ElevenLabs.Model,
	})
	if err != nil {
		log.Fatal(err)
	}
	ttsStage.Name = "tts"
	filterTTS := generationfilter.NewStage(generationfilter.Config{Generation: generationStore})
	filterTTS.Name = "generationfilter-tts"
	schedulerStage := scheduler.NewStage(scheduler.Config{})
	schedulerStage.Name = "scheduler"
	filterScheduler := generationfilter.NewStage(generationfilter.Config{Generation: generationStore})
	filterScheduler.Name = "generationfilter-scheduler"
	routerStage := router.NewStage(router.Config{})
	routerStage.Name = "router"
	collectorStage, collected := newCollector()

	g := graph.New()
	utteranceNode := g.AddNode(utterance)
	committerNode := g.AddNode(committer)
	llmNode := g.AddNode(llmStage)
	filterLLMNode := g.AddNode(filterLLM)
	ttsNode := g.AddNode(ttsStage)
	filterTTSNode := g.AddNode(filterTTS)
	schedulerNode := g.AddNode(schedulerStage)
	filterSchedulerNode := g.AddNode(filterScheduler)
	routerNode := g.AddNode(routerStage)
	collectorNode := g.AddNode(collectorStage)

	g.ConnectKinds(utteranceNode, committerNode, types.EventConversationCommitRequest)
	g.ConnectKinds(committerNode, llmNode, types.EventLLMRequest)
	g.ConnectKinds(committerNode, collectorNode, types.EventRealtimeOutput)
	g.ConnectKinds(llmNode, filterLLMNode, types.EventTimelineItem)
	g.ConnectKinds(filterLLMNode, ttsNode, types.EventTimelineItem)
	g.ConnectKinds(ttsNode, filterTTSNode, types.EventTimelineItem, types.EventPlayableSpeech)
	g.ConnectKinds(filterTTSNode, schedulerNode, types.EventTimelineItem, types.EventPlayableSpeech)
	g.ConnectKinds(schedulerNode, filterSchedulerNode, types.EventScheduledItem)
	g.ConnectKinds(filterSchedulerNode, routerNode, types.EventScheduledItem)
	g.ConnectKinds(routerNode, collectorNode, types.EventRealtimeAudio)
	g.ConnectKinds(routerNode, committerNode, types.EventConversationCommitRequest)

	go func() {
		if err := g.Run(ctx); err != nil {
			log.Printf("graph run error: %v", err)
		}
	}()

	return utterance, collected, func() {
		if err := g.Close(); err != nil {
			log.Printf("graph close error: %v", err)
		}
	}
}

func waitForResponse(ctx context.Context, collected <-chan types.Event) error {
	var sawUser bool
	var sawAgent bool
	var sawAudio bool
	for !(sawUser && sawAgent && sawAudio) {
		select {
		case evt := <-collected:
			switch evt.Kind {
			case types.EventRealtimeOutput:
				line := evt.Payload.(types.OutputLine)
				if line.Role == types.RoleUser {
					sawUser = true
					fmt.Printf("USER_TEXT=%s\n", line.Text)
				}
				if line.Role == types.RoleAgent {
					sawAgent = true
					fmt.Printf("AGENT_TEXT=%s\n", line.Text)
				}
			case types.EventRealtimeAudio:
				audio := evt.Payload.(types.OutputAudio)
				if audio.Audio != "" {
					sawAudio = true
					fmt.Printf("AUDIO_BYTES_BASE64=%d\n", len(audio.Audio))
				}
			}
		case <-ctx.Done():
			return fmt.Errorf("timeout: user=%t agent=%t audio=%t: %w", sawUser, sawAgent, sawAudio, ctx.Err())
		}
	}
	return nil
}
