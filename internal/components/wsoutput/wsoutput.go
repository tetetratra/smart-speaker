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
	clients  *sync.Map // map[*websocket.Conn]struct{}
	once     sync.Once
}

type OutMessage struct {
	Type  string `json:"type"`
	Audio string `json:"audio"`
	Role  string `json:"role,omitempty"`
}

// NewStage は指定アドレスで WS サーバーを立て、EventRealtimeAudio をクライアントへ送信する Stage を返す。
func NewStage(clients *sync.Map) *graph.Stage {
	w := &WSOutput{
		upstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		clients:  clients,
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
			w.clients.Range(func(key, _ any) bool {
				conn := key.(*websocket.Conn)
				conn.Write(ctx, websocket.MessageText, data)
				return true
			})
		}
	}
}

func (w *WSOutput) close() error {
	var err error
	w.once.Do(func() {
		close(w.upstream)
		if w.clients != nil {
			w.clients.Range(func(key, _ any) bool {
				conn := key.(*websocket.Conn)
				conn.Close(websocket.StatusNormalClosure, "bye")
				return true
			})
		}
	})
	return err
}
