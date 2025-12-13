package wsoutput

import (
	"context"
	"encoding/json"
	"sync"

	"nhooyr.io/websocket"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

// WSOutput は EventRealtimeAudio を WebSocket クライアントに送り出す。
type WSOutput struct {
	upstream chan types.Event
	conn     *websocket.Conn
	once     sync.Once
}

type OutMessage struct {
	Type  string `json:"type"`
	Audio string `json:"audio"`
	Role  string `json:"role,omitempty"`
}

// NewStage は EventRealtimeAudio をクライアントへ送信する Stage を返す。
func NewStage() *graph.Stage {
	w := &WSOutput{
		upstream: make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	return &graph.Stage{
		Upstream: w.upstream,
		Run:      w.run,
		CloseFn:  w.close,
	}
}

func (w *WSOutput) run(ctx context.Context) {
	go w.consume(ctx)
}

func (w *WSOutput) consume(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-w.upstream:
			if !ok {
				return
			}
			if evt.Kind != types.EventRealtimeAudio {
				continue
			}
			audio, ok := evt.Payload.(types.OutputAudio)
			if !ok {
				continue
			}
			msg := OutMessage{
				Type:  "audio.play",
				Audio: audio.Audio,
				Role:  audio.Role,
			}
			data, _ := json.Marshal(msg)
			if w.conn != nil {
				w.conn.Write(ctx, websocket.MessageText, data)
			}
		}
	}
}

func (w *WSOutput) close() error {
	var err error
	w.once.Do(func() {
		close(w.upstream)
		if w.conn != nil {
			w.conn.Close(websocket.StatusNormalClosure, "bye")
			w.conn = nil
		}
	})
	return err
}
