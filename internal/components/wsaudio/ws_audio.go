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

// NewStages sets up a single WS endpoint (/ws/audio) that accepts audio.append from the client
// and allows sending audio.play back to the same connection. Returns (inputStage, outputStage).
func NewStages(server *http.Server, mux *http.ServeMux) (*graph.Stage, *graph.Stage) {
	holder := &connHolder{}
	in := newInputStage(server, mux, holder)
	out := newOutputStage(holder)
	return in, out
}

// ------- input -------

type wsInput struct {
	downstream chan types.Event
	server     *http.Server
	holder     *connHolder
	once       sync.Once
}

type inMessage struct {
	Type  string `json:"type"`
	Audio string `json:"audio,omitempty"`
}

func newInputStage(server *http.Server, mux *http.ServeMux, holder *connHolder) *graph.Stage {
	w := &wsInput{
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		server:     server,
		holder:     holder,
	}
	mux.HandleFunc("/ws/audio", w.handleWS)
	return &graph.Stage{
		Downstream: w.downstream,
		Run:        w.run,
		CloseFn:    w.close,
	}
}

func (w *wsInput) run(ctx context.Context) {
	go func() {
		if err := w.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("wsaudio listen error: %v", err)
		}
	}()
}

func (w *wsInput) close() error {
	var err error
	w.once.Do(func() {
		close(w.downstream)
		if w.server != nil {
			err = w.server.Close()
		}
		if w.holder != nil {
			w.holder.clear()
		}
	})
	return err
}

func (w *wsInput) handleWS(rw http.ResponseWriter, r *http.Request) {
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
		_, data, err := c.Read(r.Context())
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("wsaudio read error: %v", err)
			}
			return
		}
		var msg inMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("wsaudio json error: %v", err)
			continue
		}
		if msg.Type != "audio.append" || msg.Audio == "" {
			continue
		}
		log.Printf("wsinput recv len=%d", len(msg.Audio))
		evt := types.Event{Kind: types.EventAudioChunk, Payload: types.AudioChunk(msg.Audio)}
		select {
		case w.downstream <- evt:
		case <-r.Context().Done():
			return
		}
	}
}

// ------- output -------

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

func newOutputStage(holder *connHolder) *graph.Stage {
	w := &wsOutput{
		upstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		holder:   holder,
	}
	return &graph.Stage{
		Upstream: w.upstream,
		Run:      w.run,
		CloseFn:  w.close,
	}
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
