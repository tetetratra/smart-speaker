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
)

// Config holds connection settings for the OpenAI Realtime API.
type Config struct {
	APIKey             string
	Model              string
	TranscriptionModel string
	Instructions       string
	Voice              string
	Modalities         []string
	DebugPrintMsgType  bool
	DebugDumpResponses bool
	ElevenLabs         ElevenLabsConfig
}

type ElevenLabsConfig struct {
	APIKey  string
	VoiceID string
	Model   string
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
	conn.SetReadLimit(4 * 1024 * 1024)

	client := &Client{ctx: ctx, conn: conn, config: cfg}

	log.Printf("✅ Connected to %s", cfg.Model)
	return client, nil
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

// Send marshals payload as JSON and writes it to the websocket.
func (c *Client) Send(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	return c.conn.Write(writeCtx, websocket.MessageText, data)
}
