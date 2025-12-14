package wschat

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

// NewStage registers /ws/chat on the provided mux and returns a stage that
// pushes text/tool events to the connected client.
func NewStage(mux *http.ServeMux) *graph.Stage {
	holder := &connHolder{}
	c := &chatWS{
		upstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		holder:   holder,
	}
	mux.HandleFunc("/ws/chat", c.handleWS)
	return &graph.Stage{
		Upstream: c.upstream,
		Run:      c.run,
		CloseFn:  c.close,
	}
}

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

type chatWS struct {
	upstream chan types.Event
	holder   *connHolder
	once     sync.Once
}

func (c *chatWS) run(ctx context.Context) {
	go c.consume(ctx)
}

func (c *chatWS) consume(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-c.upstream:
			if !ok {
				return
			}
			c.handleEvent(ctx, evt)
		}
	}
}

func (c *chatWS) handleEvent(ctx context.Context, evt types.Event) {
	msg := map[string]any{}
	switch evt.Kind {
	case types.EventRealtimeOutput:
		line, ok := evt.Payload.(types.OutputLine)
		if !ok {
			return
		}
		msg = map[string]any{
			"type":        "message",
			"role":        line.Role,
			"text":        line.Text,
			"response_id": line.ResponseID,
			"final":       line.Final,
		}
	case types.EventTextInput:
		line, ok := evt.Payload.(types.OutputLine)
		if !ok {
			return
		}
		msg = map[string]any{
			"type": "message",
			"role": line.Role,
			"text": line.Text,
		}
	case types.EventToolRequest:
		req, ok := evt.Payload.(types.ToolRequest)
		if !ok {
			return
		}
		msg = map[string]any{
			"type":         "function_call",
			"tool_call_id": req.ToolCallID,
			"name":         req.Name,
			"arguments":    json.RawMessage(req.Arguments),
			"response_id":  req.ResponseID,
		}
	case types.EventToolResponse:
		resp, ok := evt.Payload.(types.ToolResponse)
		if !ok {
			return
		}
		msg = map[string]any{
			"type":         "function_result",
			"tool_call_id": resp.ToolCallID,
			"output":       json.RawMessage(resp.Output),
		}
	default:
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("wschat: marshal error: %v", err)
		return
	}
	if conn := c.holder.get(); conn != nil {
		_ = conn.Write(ctx, websocket.MessageText, data)
	}
}

func (c *chatWS) handleWS(rw http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		log.Printf("wschat accept error: %v", err)
		return
	}
	if c.holder != nil {
		c.holder.swap(conn)
	}
	defer func() {
		if c.holder != nil {
			c.holder.clear()
		}
	}()
	for {
		if _, _, err := conn.Read(r.Context()); err != nil {
			if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
				log.Printf("wschat read error: %v", err)
			}
			return
		}
	}
}

func (c *chatWS) close() error {
	var err error
	c.once.Do(func() {
		close(c.upstream)
		if c.holder != nil {
			c.holder.clear()
		}
	})
	return err
}
