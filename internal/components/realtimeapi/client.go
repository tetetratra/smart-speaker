package realtimeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"

	types "smart-speaker/internal/types"
)

// Config holds connection settings for the OpenAI Realtime API.
type Config struct {
	APIKey             string
	Model              string
	TranscriptionModel string
	Instructions       string
}

// OpenAI Realtime API への同期的なアクセスをまとめる
type Client struct {
	ctx       context.Context
	conn      *websocket.Conn
	config    Config
	closeOnce sync.Once
}

// Realtime エンドポイントに接続してセッションを初期化する
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	wsURL := fmt.Sprintf("wss://api.openai.com/v1/realtime?model=%s", cfg.Model)

	header := http.Header{}
	header.Set("Authorization", "Bearer "+cfg.APIKey)
	header.Set("OpenAI-Beta", "realtime=v1")

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return nil, err
	}

	client := &Client{ctx: ctx, conn: conn, config: cfg}

	if err := client.sendSessionUpdate(); err != nil {
		conn.Close(websocket.StatusInternalError, "session init failed")
		return nil, err
	}

	log.Printf("✅ Connected to %s", cfg.Model)
	return client, nil
}

// 音声チャンクを API に送信する
func (c *Client) Process(ctx context.Context, chunk types.AudioChunk) error {
	payload := wsMessage{
		"type":  "input_audio_buffer.append",
		"audio": string(chunk),
	}
	return c.send(payload)
}

// API から受信した出力行を 1 件ずつ返す
func (c *Client) Read(ctx context.Context) ([]byte, error) {
	_, data, err := c.conn.Read(ctx)
	return data, err
}

// WebSocket 接続を閉じる
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		if c.conn != nil {
			err = c.conn.Close(websocket.StatusNormalClosure, "closing")
		}
	})
	return err
}

func (c *Client) send(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	return c.conn.Write(writeCtx, websocket.MessageText, data)
}

func (c *Client) Send(payload any) error {
	return c.send(payload)
}

func (c *Client) sendSessionUpdate() error {
	session := wsMessage{
		"instructions":       c.config.Instructions,
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
	if c.config.TranscriptionModel != "" {
		session["input_audio_transcription"] = map[string]any{
			"model":    c.config.TranscriptionModel,
			"language": "ja",
			"prompt":   "web系のITエンジニアが話しており、専門用語が来ることを想定してください。日本語で短い応答を心がけてください",
		}
	}

	payload := wsMessage{
		"type":    "session.update",
		"session": session,
	}
	return c.send(payload)
}

type wsMessage map[string]any
