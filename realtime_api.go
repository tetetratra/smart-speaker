package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	toolsvc "smart-speaker/tools"

	"nhooyr.io/websocket"
)

type RealtimeAPI struct {
	ctx          context.Context
	cancel       context.CancelFunc
	conn         *websocket.Conn
	config       config
	voiceIn      <-chan string
	responseOut  chan<- OutputLine
	once         sync.Once
	toolMu       sync.Mutex
	toolStates   map[string]*toolCallState
	switchBotMu  sync.Once
	switchBot    *toolsvc.SwitchBotClient
	switchBotErr error
}

func NewRealtimeAPI(ctx context.Context, cfg config, voice <-chan string, responses chan<- OutputLine) (*RealtimeAPI, error) {
	wsURL := fmt.Sprintf("wss://api.openai.com/v1/realtime?model=%s", cfg.Model)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+cfg.APIKey)
	header.Set("OpenAI-Beta", "realtime=v1")

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(ctx)

	api := &RealtimeAPI{
		ctx:         runCtx,
		cancel:      cancel,
		conn:        conn,
		config:      cfg,
		voiceIn:     voice,
		responseOut: responses,
		toolStates:  make(map[string]*toolCallState),
	}

	if err := api.sendSessionUpdate(); err != nil {
		cancel()
		conn.Close(websocket.StatusInternalError, "session init failed")
		return nil, err
	}

	log.Printf("✅ Connected to %s", cfg.Model)
	return api, nil
}

func (api *RealtimeAPI) Run() {
	api.once.Do(func() {
		go api.sendLoop()
		go api.receiveLoop()
	})
}

func (api *RealtimeAPI) sendSessionUpdate() error {
	instructions := loadSystemPrompt()
	session := map[string]any{
		"instructions":       instructions,
		"modalities":         []string{"text"},
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
				"name":        "get_current_time",
				"description": "現在の日時を ISO8601 形式で返します。引数は受け付けません。",
			},
			map[string]any{
				"type":        "function",
				"name":        "get_weather",
				"description": "現在の天気を返します。レスポンスは数秒遅れて到着します。引数は受け付けません。",
			},
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
		},
	}
	if api.config.TranscriptionModel != "" {
		// MEMO: この文字起こしがAIに渡るわけではないが、精度はマシなので、いったんこっちを挟むのもあり
		session["input_audio_transcription"] = map[string]any{
			"model":    api.config.TranscriptionModel,
			"language": "ja",
			"prompt":   "web系のITエンジニアが話しており、専門用語が来ることを想定してください。日本語で短い応答を心がけてください",
		}
	}
	payload := wsMessage{
		"type":    "session.update",
		"session": session,
	}
	return api.send(payload)
}

func (api *RealtimeAPI) send(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(api.ctx, 5*time.Second)
	defer cancel()
	return api.conn.Write(writeCtx, websocket.MessageText, data)
}

func (api *RealtimeAPI) sendLoop() {
	for {
		select {
		case <-api.ctx.Done():
			return
		case msg, ok := <-api.voiceIn:
			if !ok {
				return
			}
			payload := wsMessage{
				"type":  "input_audio_buffer.append",
				"audio": msg,
			}
			if err := api.send(payload); err != nil {
				log.Printf("write error: %v", err)
				return
			}
		}
	}
}

func (api *RealtimeAPI) receiveLoop() {
	defer close(api.responseOut)
	for {
		_, data, err := api.conn.Read(api.ctx)
		if err != nil {
			status := websocket.CloseStatus(err)
			if status != websocket.StatusNormalClosure && status != -1 {
				log.Printf("read error: %v", err)
			}
			return
		}

		var msg wsMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("unmarshal error: %v", err)
			continue
		}

		if api.handleToolMessage(msg) {
			continue
		}

		lines := parseMessageForLines(msg)
		for _, line := range lines {
			select {
			case <-api.ctx.Done():
				return
			case api.responseOut <- line:
			}
		}
	}
}

func (api *RealtimeAPI) Close() {
	api.cancel()
	api.conn.Close(websocket.StatusNormalClosure, "closing")
}

func loadSystemPrompt() string {
	data, err := os.ReadFile("system_prompt.txt")
	if err != nil {
		log.Printf("warning: system_prompt.txt not found. (%v)", err)
		return ""
	}
	return string(data)
}
