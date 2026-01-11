package sender

import (
	"context"
	"log"

	types "smart-speaker/internal/types"
)

// Client defines the methods required by the sender runner.
type Client interface {
	Send(any) error
}

// SessionConfig holds values required to build the initial session update payload.
type SessionConfig struct {
	Instructions       string
	Voice              string
	Modalities         []string
	TranscriptionModel string
}

// Runner pulls events from the upstream channel and forwards them to the Realtime API.
type Runner struct {
	ctx         context.Context
	upstream    <-chan types.Event
	handler     *EventHandler
	sessionInfo SessionConfig
}

func NewRunner(ctx context.Context, client Client, upstream <-chan types.Event, sessionInfo SessionConfig) *Runner {
	return &Runner{
		ctx:         ctx,
		upstream:    upstream,
		handler:     NewEventHandler(ctx, client, sessionInfo.Voice),
		sessionInfo: sessionInfo,
	}
}

func (r *Runner) Run() {
	log.Printf("realtime sender started")
	if err := sendSessionUpdate(r.handler.client, r.sessionInfo); err != nil {
		log.Printf("realtime session update error: %v", err)
	}
	for {
		select {
		case <-r.ctx.Done():
			return
		case evt, ok := <-r.upstream:
			if !ok {
				return
			}
			r.handler.Handle(evt)
		}
	}
}

func sendSessionUpdate(client Client, cfg SessionConfig) error {
	session := map[string]any{
		"instructions": cfg.Instructions,
		"modalities":   []string{"text"},
		// 音声入力は常に PCM16 を送るためフォーマット指定
		"input_audio_format": "pcm16",
		"turn_detection": map[string]any{
			"type":               "semantic_vad",
			"eagerness":          "low",
			"create_response":    false,
			"interrupt_response": false,
		},
	}
	if cfg.TranscriptionModel != "" {
		session["input_audio_transcription"] = map[string]any{
			"model":    cfg.TranscriptionModel,
			"language": "ja",
		}
	}
	return client.Send(map[string]any{
		"type":    "session.update",
		"session": session,
	})
}
