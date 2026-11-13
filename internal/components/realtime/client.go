package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tetetratra/smart-speaker/internal/eventqueue"
	"github.com/tetetratra/smart-speaker/internal/types"
	"net/http"
	"net/url"
	"nhooyr.io/websocket"
	"time"
)

// transport は一接続の送受信。送信待ちは受信側や呼び出し元へ伝えない。
type transport struct {
	conn     *websocket.Conn
	commands *eventqueue.Queue[any]
	cancel   context.CancelFunc
	epoch    uint64
}
type incoming struct {
	history     []types.ConversationRecord
	memory      string
	excluded    map[string]bool
	restore     bool
	epoch       uint64
	data        []byte
	conn        *transport
	err         error
	event       any
	context     string
	restoredIDs []string
}

func dial(ctx context.Context, cfg Config, epoch uint64) (*transport, error) {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = "wss://api.openai.com/v1/realtime"
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("model", cfg.Model)
	u.RawQuery = q.Encode()
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, u.String(), &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": []string{"Bearer " + cfg.APIKey}}})
	if err != nil {
		return nil, fmt.Errorf("Realtime接続: %w", err)
	}
	conn.SetReadLimit(16 << 20)
	return &transport{conn: conn, commands: eventqueue.New[any](nil), epoch: epoch}, nil
}
func (t *transport) run(parent context.Context, events *eventqueue.Queue[incoming]) {
	ctx, cancel := context.WithCancel(parent)
	t.cancel = cancel
	go func() {
		defer cancel()
		for {
			v, ok := t.commands.Pop(ctx)
			if !ok {
				return
			}
			data, err := json.Marshal(v)
			if err != nil {
				events.Push(incoming{epoch: t.epoch, err: err})
				return
			}
			writeCtx, done := context.WithTimeout(ctx, 10*time.Second)
			err = t.conn.Write(writeCtx, websocket.MessageText, data)
			done()
			if err != nil {
				events.Push(incoming{epoch: t.epoch, err: err})
				t.conn.Close(websocket.StatusNormalClosure, "")
				return
			}
		}
	}()
	go func() {
		defer cancel()
		for {
			_, data, err := t.conn.Read(ctx)
			if err != nil {
				events.Push(incoming{epoch: t.epoch, err: err})
				return
			}
			events.Push(incoming{epoch: t.epoch, data: data})
		}
	}()
}
func (t *transport) send(v any) { t.commands.Push(v) }
func (t *transport) close() {
	if t.cancel != nil {
		t.cancel()
	}
	t.conn.Close(websocket.StatusNormalClosure, "")
}

func sessionUpdate(cfg Config, instructions string) map[string]any {
	turnDetection := map[string]any{"type": cfg.TurnDetection, "create_response": false, "interrupt_response": cfg.WakeMode == nil || !cfg.WakeMode.Snapshot(time.Now()).Required}
	if cfg.TurnDetection == "server_vad" {
		turnDetection["silence_duration_ms"] = 800
	} else {
		turnDetection["eagerness"] = "auto"
	}
	return map[string]any{"type": "session.update", "session": map[string]any{
		"type": "realtime", "output_modalities": []string{"text"}, "instructions": instructions, "tools": cfg.Tools, "tool_choice": "auto",
		"audio": map[string]any{"input": map[string]any{
			"format":          map[string]any{"type": "audio/pcm", "rate": 24000},
			"noise_reduction": map[string]any{"type": "far_field"},
			"transcription":   map[string]any{"model": cfg.TranscriptionModel, "language": "ja"},
			"turn_detection":  turnDetection,
		}},
	}}
}
