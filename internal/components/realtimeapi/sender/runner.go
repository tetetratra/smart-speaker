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
		"modalities":   []string{"text", "audio"},
		// 音声入力は常に PCM16 を送るためフォーマット指定
		"input_audio_format": "pcm16",
		"turn_detection": map[string]any{
			"type":                "server_vad",
			"threshold":           0.65,
			"silence_duration_ms": 800,
			"interrupt_response":  false,
		},
		"tools": []any{
			map[string]any{
				"type":        "function",
				"name":        "switchbot_control_device",
				"description": "SwitchBot API を使ってデバイスを操作します。",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"device_id": map[string]any{
							"type":        "string",
							"description": "SwitchBot デバイスの ID",
						},
						"device": map[string]any{
							"type":        "string",
							"description": "環境変数 SWITCHBOT_DEVICE_MAP で定義したエイリアス名",
						},
						"command": map[string]any{
							"type":        "string",
							"description": "SwitchBot API の command (例: turnOn, turnOff, press)",
						},
						"parameter": map[string]any{
							"type":        "string",
							"description": "必要に応じて command に渡す parameter。不要なら default。",
						},
						"command_type": map[string]any{
							"type":        "string",
							"description": "SwitchBot API の commandType。省略時は command。",
						},
					},
					"required": []string{"command"},
				},
			},
			map[string]any{
				"type":        "function",
				"name":        "web_fetch",
				"description": "指定した URL にアクセスし、ステータスコードと本文を返します。",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"url": map[string]any{
							"type":        "string",
							"description": "HTTP GET する URL",
						},
					},
					"required": []string{"url"},
				},
			},
		},
	}
	session["output_audio_format"] = "pcm16"
	if cfg.Voice != "" {
		session["voice"] = cfg.Voice
	}
	if cfg.TranscriptionModel != "" {
		session["input_audio_transcription"] = map[string]any{
			"model":    cfg.TranscriptionModel,
			"language": "ja",
			"prompt":   "web系のITエンジニアが話しており、専門用語が来ることを想定してください。日本語で短い応答を心がけてください",
		}
	}
	return client.Send(map[string]any{
		"type":    "session.update",
		"session": session,
	})
}

func defaultModalities(modalities []string) []string {
	return []string{"text", "audio"}
}

func hasAudio(modalities []string) bool {
	for _, m := range modalities {
		if m == "audio" {
			return true
		}
	}
	return false
}
