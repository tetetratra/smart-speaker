package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

// Config defines settings for ElevenLabs stream-input TTS.
type Config struct {
	APIKey string
	Voice  string
	Model  string
	// HTTP headers: xi-api-key is required
}

// NewStage converts EventRealtimeOutput (assistant text) stream into EventRealtimeAudio
// by using ElevenLabs stream-input WebSocket API.
func NewStage(cfg Config) (*graph.Stage, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("elevenlabs: API key is required")
	}
	if cfg.Voice == "" {
		return nil, fmt.Errorf("elevenlabs: voice id is required")
	}
	if cfg.Model == "" {
		cfg.Model = "eleven_multilingual_v2"
	}
	t := &streamTTS{
		cfg:        cfg,
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	return &graph.Stage{
		Upstream:   t.upstream,
		Downstream: t.downstream,
		Run:        t.run,
		CloseFn:    t.close,
	}, nil
}

type streamTTS struct {
	cfg        Config
	upstream   chan types.Event
	downstream chan types.Event

	mu          sync.Mutex
	conn        *websocket.Conn
	cancelRead  context.CancelFunc
	connectedID string // optional tracking of current response
}

func (t *streamTTS) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			t.closeConn()
			return
		case evt, ok := <-t.upstream:
			if !ok {
				t.closeConn()
				return
			}
			if evt.Kind != types.EventRealtimeOutput {
				continue
			}
			line, ok := evt.Payload.(types.OutputLine)
			if !ok {
				continue
			}
			if line.Role != "" && line.Role != "assistant" {
				continue
			}
			// open connection lazily
			if err := t.ensureConn(ctx, line.ResponseID); err != nil {
				log.Printf("elevenlabs: connect error: %v", err)
				continue
			}
			if line.Final {
				if err := t.sendFlush(ctx); err != nil {
					log.Printf("elevenlabs: flush error: %v", err)
				}
				t.closeConn()
				continue
			}
			if line.Text == "" {
				continue
			}
			if err := t.sendText(ctx, line.Text); err != nil {
				log.Printf("elevenlabs: send text error: %v", err)
				t.closeConn()
				continue
			}
		}
	}
}

func (t *streamTTS) ensureConn(parent context.Context, respID string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	url := fmt.Sprintf("wss://api.elevenlabs.io/v1/text-to-speech/%s/stream-input", t.cfg.Voice)
	headers := http.Header{}
	headers.Set("xi-api-key", t.cfg.APIKey)
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		return err
	}
	// send initial settings
	init := map[string]any{
		"text":     "",
		"model_id": t.cfg.Model,
	}
	_ = conn.Write(ctx, websocket.MessageText, mustJSON(init))

	readCtx, cancelRead := context.WithCancel(context.Background())
	go t.readLoop(readCtx, conn)
	t.conn = conn
	t.cancelRead = cancelRead
	t.connectedID = respID
	return nil
}

func (t *streamTTS) sendText(ctx context.Context, text string) error {
	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("connection not ready")
	}
	payload := map[string]any{
		"text": text,
	}
	return conn.Write(ctx, websocket.MessageText, mustJSON(payload))
}

func (t *streamTTS) sendFlush(ctx context.Context) error {
	t.mu.Lock()
	conn := t.conn
	t.mu.Unlock()
	if conn == nil {
		return nil
	}
	payload := map[string]any{
		"text":                   "",
		"flush":                  true,
		"try_trigger_generation": true,
	}
	return conn.Write(ctx, websocket.MessageText, mustJSON(payload))
}

func (t *streamTTS) readLoop(ctx context.Context, conn *websocket.Conn) {
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		switch typ {
		case websocket.MessageBinary:
			audioB64 := base64.StdEncoding.EncodeToString(data)
			select {
			case t.downstream <- types.Event{Kind: types.EventRealtimeAudio, Payload: types.OutputAudio{Role: "assistant", Audio: audioB64}}:
			case <-ctx.Done():
				return
			}
		case websocket.MessageText:
			// debug/info messages are ignored
		}
	}
}

func (t *streamTTS) closeConn() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancelRead != nil {
		t.cancelRead()
		t.cancelRead = nil
	}
	if t.conn != nil {
		_ = t.conn.Close(websocket.StatusNormalClosure, "bye")
		t.conn = nil
		t.connectedID = ""
	}
}

func (t *streamTTS) close() error {
	t.closeConn()
	close(t.upstream)
	close(t.downstream)
	return nil
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("elevenlabs: json marshal error: %v", err)
		return nil
	}
	return b
}
