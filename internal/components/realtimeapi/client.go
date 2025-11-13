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

// OpenAI Realtime API への同期的なアクセスをまとめる
type Client struct {
	ctx        context.Context
	conn       *websocket.Conn
	config     Config
	toolStates map[string]*toolCallState
	toolMu     sync.Mutex
	toolQueue  []types.ToolRequest

	responseDeltaSeen map[string]bool
	buffer            []types.OutputLine

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

	client := &Client{
		ctx:               ctx,
		conn:              conn,
		config:            cfg,
		toolStates:        make(map[string]*toolCallState),
		responseDeltaSeen: make(map[string]bool),
	}

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
func (c *Client) NextEvent(ctx context.Context) (types.Event, error) {
	for {
		if evt, ok := c.popToolRequest(); ok {
			return evt, nil
		}
		if len(c.buffer) > 0 {
			line := c.buffer[0]
			c.buffer = c.buffer[1:]
			return types.Event{Kind: types.EventRealtimeOutput, Payload: line}, nil
		}

		if err := ctx.Err(); err != nil {
			return types.Event{}, err
		}

		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return types.Event{}, err
		}

		var msg wsMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("unmarshal error: %v", err)
			continue
		}

		if c.handleToolMessage(msg) {
			continue
		}

		lines := c.parseMessageForLines(msg)
		if len(lines) == 0 {
			continue
		}
		c.buffer = append(c.buffer, lines...)
	}
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

func (c *Client) enqueueToolRequest(req types.ToolRequest) {
	c.toolMu.Lock()
	defer c.toolMu.Unlock()
	c.toolQueue = append(c.toolQueue, req)
}

func (c *Client) popToolRequest() (types.Event, bool) {
	c.toolMu.Lock()
	defer c.toolMu.Unlock()
	if len(c.toolQueue) == 0 {
		return types.Event{}, false
	}
	req := c.toolQueue[0]
	c.toolQueue = c.toolQueue[1:]
	return types.Event{Kind: types.EventToolRequest, Payload: req}, true
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
