package wsaudio

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"

	"nhooyr.io/websocket"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

type connHolder struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (h *connHolder) swap(c *websocket.Conn) {
	h.mu.Lock()
	if h.conn != nil && h.conn != c {
		h.conn.Close(websocket.StatusNormalClosure, "bye")
	}
	h.conn = c
	h.mu.Unlock()
}

func (h *connHolder) get() *websocket.Conn {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.conn
}

func (h *connHolder) clear() {
	h.mu.Lock()
	if h.conn != nil {
		h.conn.Close(websocket.StatusNormalClosure, "bye")
		h.conn = nil
	}
	h.mu.Unlock()
}

// NewStage は /ws/audio の音声再生専用 WS を用意します。
func NewStage(mux *http.ServeMux) *graph.Stage {
	w := &wsOutput{
		upstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		holder:   &connHolder{},
	}
	mux.HandleFunc("/ws/audio", w.handleWS)
	return &graph.Stage{
		Upstream: w.upstream,
		Run:      w.run,
		CloseFn:  w.close,
	}
}

type wsOutput struct {
	upstream chan types.Event
	holder   *connHolder
	once     sync.Once
}

type outMessage struct {
	Type  string `json:"type"`
	Audio string `json:"audio"`
	Role  string `json:"role,omitempty"`
}

func (w *wsOutput) run(ctx context.Context) {
	go w.consume(ctx)
}

func (w *wsOutput) consume(ctx context.Context) {
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
			msg := outMessage{
				Type:  "audio.play",
				Audio: audio.Audio,
				Role:  audio.Role,
			}
			data, _ := json.Marshal(msg)
			if conn := w.holder.get(); conn != nil {
				conn.Write(ctx, websocket.MessageText, data)
			}
		}
	}
}

func (w *wsOutput) handleWS(rw http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("wsaudio accept error: %v", err)
		return
	}
	if w.holder != nil {
		w.holder.swap(c)
	}
	defer c.Close(websocket.StatusNormalClosure, "bye")
	defer func() {
		if w.holder != nil {
			w.holder.clear()
		}
	}()

	for {
		_, _, err := c.Read(r.Context())
		if err != nil {
			if !errors.Is(err, context.Canceled) && websocket.CloseStatus(err) != websocket.StatusNormalClosure {
				log.Printf("wsaudio read error: %v", err)
			}
			return
		}
	}
}

func (w *wsOutput) close() error {
	var err error
	w.once.Do(func() {
		close(w.upstream)
		if w.holder != nil {
			w.holder.clear()
		}
	})
	return err
}
