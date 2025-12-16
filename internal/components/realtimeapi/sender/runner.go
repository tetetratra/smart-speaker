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
				"type": "function",
				"name": "sub_ai",
				"description": `任意の文字列クエリを受け取り、サブAIに投げて調査・回答を得ます。
				- ユーザーからの質問に対して、以下の状況の場合は積極的に活用してください
				  - 自分が正確な回答を知らない場合
				    - 例：sub_ai("明日の東京の天気を教えて")
				  - より詳しい・最新の情報が必要な場合
				    - 例：sub_ai("学マスで今やっているイベントを教えて")
				  - 難しい質問が来た場合
				    - 例：sub_ai("一般にチケット管理システムではどのようにチケットをモデリングするのが良いとされていますか")`,
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{
							"type":        "string",
							"description": "サブAIに渡す質問や調査依頼のテキスト",
						},
					},
					"required": []string{"query"},
				},
			},
			map[string]any{
				"type":        "function",
				"name":        "schedule_timer",
				"description": "指定時刻にリマインドをセットします。role=system のメッセージとして再度送ります。",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"type": map[string]any{
							"type":        "string",
							"enum":        []string{"absolute", "relative"},
							"description": "absolute または relative を指定してください。",
						},
						"description": map[string]any{
							"type":        "string",
							"description": "その時間に知らせたい内容（短め）。",
						},
						"minutes": map[string]any{
							"type":        "integer",
							"description": "relative の場合、何分後か（整数）。",
						},
						"year": map[string]any{
							"type":        "integer",
							"description": "absolute の場合の年（西暦）。",
						},
						"month": map[string]any{
							"type":        "integer",
							"description": "absolute の場合の月（1-12）。",
						},
						"day": map[string]any{
							"type":        "integer",
							"description": "absolute の場合の日（1-31）。",
						},
						"hour": map[string]any{
							"type":        "integer",
							"description": "absolute の場合の時（0-23）。",
						},
						"minute": map[string]any{
							"type":        "integer",
							"description": "absolute の場合の分（0-59）。",
						},
					},
					"required": []string{"type", "description"},
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
		}
	}
	return client.Send(map[string]any{
		"type":    "session.update",
		"session": session,
	})
}
