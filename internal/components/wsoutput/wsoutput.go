package wsoutput

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"nhooyr.io/websocket"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

// WSOutput は EventRealtimeAudio を WebSocket クライアントに送り出す。
type WSOutput struct {
	upstream chan types.Event
	server   *http.Server
	clients  sync.Map // map[*websocket.Conn]struct{}
	once     sync.Once
}

type OutMessage struct {
	Type  string `json:"type"`
	Audio string `json:"audio"`
	Role  string `json:"role,omitempty"`
}

// NewStage は指定アドレスで WS サーバーを立て、EventRealtimeAudio をクライアントへ送信する Stage を返す。
func NewStage(addr string) *graph.Stage {
	w := &WSOutput{
		upstream: make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/audio", w.handleWS)
	w.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	return &graph.Stage{
		Upstream: w.upstream,
		Run:      w.run,
		CloseFn:  w.close,
	}
}

func (w *WSOutput) run(ctx context.Context) {
	go func() {
		if err := w.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("wsoutput listen error: %v", err)
		}
	}()
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

func (w *WSOutput) handleWS(rw http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("wsoutput accept error: %v", err)
		return
	}
	w.clients.Store(c, struct{}{})
	defer func() {
		w.clients.Delete(c)
		c.Close(websocket.StatusNormalClosure, "bye")
	}()

	// keep connection alive
	for {
		if _, _, err := c.Read(r.Context()); err != nil {
			return
		}
	}
}

func (w *WSOutput) close() error {
	var err error
	w.once.Do(func() {
		close(w.upstream)
		if w.server != nil {
			err = w.server.Close()
		}
		w.clients.Range(func(key, _ any) bool {
			conn := key.(*websocket.Conn)
			conn.Close(websocket.StatusNormalClosure, "bye")
			return true
		})
	})
	return err
}
