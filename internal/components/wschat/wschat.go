package wschat

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"smart-speaker/internal/graph"
	"smart-speaker/internal/state"
	types "smart-speaker/internal/types"
)

// NewStage registers /ws/chat on the provided mux and returns a stage that
// pushes text/tool events to the connected client, and also receives text
// messages from the client to emit as EventTextInput.
func NewStage(mux *http.ServeMux) *graph.Stage {
	holder := &connHolder{}
	c := &chatWS{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		holder:     holder,
	}
	mux.HandleFunc("/ws/chat", c.handleWS)
	return &graph.Stage{
		Upstream:   c.upstream,
		Downstream: c.downstream,
		Run:        c.run,
		CloseFn:    c.close,
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
	upstream   chan types.Event
	downstream chan types.Event
	holder     *connHolder
	once       sync.Once
	ctx        context.Context
	cancel     context.CancelFunc
	closerWG   sync.WaitGroup
}

func (c *chatWS) run(ctx context.Context) {
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.closerWG.Add(1)
	go func() {
		defer c.closerWG.Done()
		c.consume(c.ctx)
	}()
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
		if line.Source != "" {
			msg["source"] = line.Source
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
		if line.Source != "" {
			msg["source"] = line.Source
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
			"name":         resp.Name,
			"output":       json.RawMessage(resp.Output),
		}
	case types.EventRTCSignal:
		sig, ok := evt.Payload.(types.RTCSignal)
		if !ok {
			return
		}
		msg = map[string]any{
			"type":      sig.Type,
			"sdp":       sig.SDP,
			"candidate": sig.Candidate,
		}
	default:
		return
	}

	c.writeMessage(ctx, msg)
}

func (c *chatWS) writeMessage(ctx context.Context, msg map[string]any) {
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
		_, data, err := conn.Read(r.Context())
		if err != nil {
			if websocket.CloseStatus(err) != websocket.StatusNormalClosure {
				log.Printf("wschat read error: %v", err)
			}
			return
		}
		var msg struct {
			Type       string                 `json:"type"`
			Text       string                 `json:"text"`
			Role       string                 `json:"role"`
			Present    string                 `json:"present"`
			CapturedAt string                 `json:"captured_at"`
			Source     string                 `json:"source"`
			SDP        string                 `json:"sdp"`
			Candidate  *types.RTCIceCandidate `json:"candidate"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("wschat client message parse error: %v", err)
			continue
		}
		if strings.HasPrefix(msg.Type, "webrtc.") {
			sig := types.RTCSignal{
				Type:      msg.Type,
				SDP:       msg.SDP,
				Candidate: msg.Candidate,
			}
			select {
			case c.downstream <- types.Event{Kind: types.EventRTCSignal, Payload: sig}:
			case <-r.Context().Done():
				return
			case <-c.ctx.Done():
				return
			}
			continue
		}
		if msg.Type == "reset" {
			select {
			case c.downstream <- types.Event{Kind: types.EventReset}:
			case <-r.Context().Done():
				return
			case <-c.ctx.Done():
				return
			}
			continue
		}
		if msg.Type == "stt_start" || msg.Type == "stt_end" {
			capturedAt := time.Now()
			if ts := strings.TrimSpace(msg.CapturedAt); ts != "" {
				if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
					capturedAt = parsed
				}
			}
			kind := types.EventSpeechStart
			if msg.Type == "stt_end" {
				kind = types.EventSpeechEnd
			}
			select {
			case c.downstream <- types.Event{Kind: kind, Payload: types.SpeechEvent{Source: strings.TrimSpace(msg.Source), CapturedAt: capturedAt}}:
			case <-r.Context().Done():
				return
			case <-c.ctx.Done():
				return
			}
			continue
		}
		if msg.Type != "message" {
			continue
		}
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			continue
		}
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "user"
		}
		state.SetLastActivityAt(time.Now())
		select {
		case c.downstream <- types.Event{Kind: types.EventTextInput, Payload: types.OutputLine{Role: role, Text: text}}:
		case <-r.Context().Done():
			return
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *chatWS) close() error {
	var err error
	c.once.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		c.closerWG.Wait()
		close(c.upstream)
		close(c.downstream)
		if c.holder != nil {
			c.holder.clear()
		}
	})
	return err
}
