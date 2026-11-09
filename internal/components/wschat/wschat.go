package wschat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"

	"github.com/tetetratra/smart-speaker/internal/graph"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type Config struct {
}

// NewStage registers /ws/chat on the provided mux and returns a stage that
// pushes graph events to the connected client, and receives WebRTC signaling
// messages from the client.
func NewStage(mux *http.ServeMux, cfg Config) *graph.Stage {
	holder := &connHolder{}
	c := &chatWS{
		upstream:   make(chan types.Event),
		downstream: make(chan types.Event),
		holder:     holder,
	}
	mux.HandleFunc("/ws/chat", c.handleWS)
	c.node = &graph.Stage{
		Upstream:   c.upstream,
		Downstream: c.downstream,
		Run:        c.run,
		CloseFn:    c.close,
	}
	return c.node
}

type connHolder struct {
	mu    sync.RWMutex
	conns map[string]*websocket.Conn
}

func (h *connHolder) add(id string, c *websocket.Conn) {
	h.mu.Lock()
	if h.conns == nil {
		h.conns = map[string]*websocket.Conn{}
	}
	h.conns[id] = c
	h.mu.Unlock()
}

func (h *connHolder) remove(id string) {
	h.mu.Lock()
	if h.conns != nil {
		if conn, ok := h.conns[id]; ok {
			conn.Close(websocket.StatusNormalClosure, "bye")
			delete(h.conns, id)
		}
	}
	h.mu.Unlock()
}

func (h *connHolder) get(id string) *websocket.Conn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.conns == nil {
		return nil
	}
	return h.conns[id]
}

func (h *connHolder) snapshot() map[string]*websocket.Conn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.conns == nil {
		return nil
	}
	out := make(map[string]*websocket.Conn, len(h.conns))
	for id, conn := range h.conns {
		out[id] = conn
	}
	return out
}

func (h *connHolder) clearAll() {
	h.mu.Lock()
	if h.conns != nil {
		for id, conn := range h.conns {
			conn.Close(websocket.StatusNormalClosure, "bye")
			delete(h.conns, id)
		}
	}
	h.mu.Unlock()
}

type chatWS struct {
	node       *graph.Stage
	upstream   chan types.Event
	downstream chan types.Event
	holder     *connHolder
	once       sync.Once
	ctx        context.Context
	cancel     context.CancelFunc
	closerWG   sync.WaitGroup
	nextConnID uint64
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
		evt, ok := c.node.Receive(ctx)
		if !ok {
			return
		}
		c.handleEvent(ctx, evt)
	}

}

func (c *chatWS) handleEvent(ctx context.Context, evt types.Event) {
	msg, targetID, ok := messageForEvent(evt)
	if !ok {
		return
	}
	c.writeMessage(ctx, msg, targetID)
}

func messageForEvent(evt types.Event) (map[string]any, string, bool) {
	msg := map[string]any{}
	targetID := ""
	switch evt.Kind {
	case types.EventAudioSent:
		p, ok := evt.Payload.(types.AudioFrame)
		if !ok || (!p.End && p.Sequence != 0) {
			return nil, "", false
		}
		// 音声はWebRTCで送り、画面には区間の開始と終了だけ通知する。
		msg = map[string]any{"type": "playback_progress", "generation_id": uint64(p.GenerationID), "segment_id": p.SegmentID, "text": p.Text, "end": p.End}
		if p.SegmentDuration > 0 {
			msg["duration_ms"] = p.SegmentDuration.Milliseconds()
		}
	case types.EventGenerationChanged:
		p, ok := evt.Payload.(types.GenerationChanged)
		if !ok {
			return nil, "", false
		}
		msg = map[string]any{"type": "playback_stop", "generation_id": uint64(p.GenerationID)}
	case types.EventRealtimeStatus:
		p, ok := evt.Payload.(types.RealtimeStatus)
		if !ok {
			return nil, "", false
		}
		msg = map[string]any{"type": "realtime_status", "status": p.Status, "message": p.Message}

	case types.EventRealtimeOutput:
		line, ok := evt.Payload.(types.OutputLine)
		if !ok {
			return nil, "", false
		}
		msg = map[string]any{
			"type":          "message",
			"role":          line.Role,
			"text":          line.Text,
			"response_id":   line.ResponseID,
			"generation_id": uint64(line.GenerationID),
			"final":         line.Final,
		}
		if line.Source != "" {
			msg["source"] = line.Source
		}
	case types.EventRTCSignal:
		sig, ok := evt.Payload.(types.RTCSignal)
		if !ok {
			return nil, "", false
		}
		targetID = sig.ClientID
		msg = map[string]any{
			"type":      sig.Type,
			"sdp":       sig.SDP,
			"candidate": sig.Candidate,
		}
	case types.EventSpeechStart:
		speech, ok := evt.Payload.(types.SpeechEvent)
		if !ok {
			return nil, "", false
		}
		msg = map[string]any{
			"type":        "speech_start",
			"source":      speech.Source,
			"captured_at": speech.CapturedAt.Format(time.RFC3339Nano),
		}
	case types.EventSpeechEnd:
		speech, ok := evt.Payload.(types.SpeechEvent)
		if !ok {
			return nil, "", false
		}
		msg = map[string]any{
			"type":        "speech_end",
			"source":      speech.Source,
			"captured_at": speech.CapturedAt.Format(time.RFC3339Nano),
		}
	case types.EventWhiteboardUpdate:
		update, ok := evt.Payload.(types.WhiteboardUpdate)
		if !ok {
			return nil, "", false
		}
		msg = map[string]any{
			"type":    "whiteboard_update",
			"content": update.Content,
		}
		if !update.UpdatedAt.IsZero() {
			// ツール履歴の started_at と同じサーバーの時計で並べられるよう、書き込み時刻をミリ秒で送る。
			msg["timestamp"] = update.UpdatedAt.UnixMilli()
		}
	case types.EventWakeTranscription:
		tr, ok := evt.Payload.(types.WakeTranscription)
		if !ok {
			return nil, "", false
		}
		// 呼びかけの見逃しを調べるための情報。画面ではログ管理画面にだけ並ぶ。
		msg = map[string]any{
			"type":       "wake_transcription",
			"peer_id":    tr.PeerID,
			"text":       tr.Text,
			"outcome":    tr.Outcome,
			"confidence": tr.Confidence,
			"similarity": tr.Similarity,
			"matched":    tr.Matched,
			"accepted":   tr.Accepted,
			"wake_only":  tr.WakeOnly,
			"awake":      tr.Awake,
		}
		if tr.Error != "" {
			msg["error"] = tr.Error
		}
	case types.EventSessionReset:
		reset, ok := evt.Payload.(types.SessionResetEvent)
		if !ok {
			return nil, "", false
		}
		msg = map[string]any{
			"type":         "session_reset",
			"requested_at": reset.RequestedAt.Format(time.RFC3339Nano),
		}
	default:
		return nil, "", false
	}
	return msg, targetID, true
}

func (c *chatWS) writeMessage(ctx context.Context, msg map[string]any, targetID string) bool {
	data, err := json.Marshal(msg)
	if err != nil || c.holder == nil {
		return false
	}
	conns := c.holder.snapshot()
	if targetID != "" {
		conns = map[string]*websocket.Conn{targetID: c.holder.get(targetID)}
	}
	sent := false
	for _, conn := range conns {
		if conn == nil {
			continue
		}
		writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := conn.Write(writeCtx, websocket.MessageText, data)
		cancel()
		if err == nil {
			sent = true
		} else {
			conn.Close(websocket.StatusNormalClosure, "")
		}
	}
	return sent
}

func (c *chatWS) handleWS(rw http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(rw, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		log.Printf("wschat accept error: %v", err)
		return
	}
	connID := fmt.Sprintf("ws-%d", atomic.AddUint64(&c.nextConnID, 1))
	if c.holder != nil {
		c.holder.add(connID, conn)
	}
	defer func() {
		if c.holder != nil {
			c.holder.remove(connID)
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
			Type      string                 `json:"type"`
			Gain      *float64               `json:"gain"`
			SDP       string                 `json:"sdp"`
			Candidate *types.RTCIceCandidate `json:"candidate"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("wschat client message parse error: %v", err)
			continue
		}
		if strings.HasPrefix(msg.Type, "webrtc.") {
			sig := types.RTCSignal{
				Type:      msg.Type,
				Gain:      msg.Gain,
				SDP:       msg.SDP,
				Candidate: msg.Candidate,
				ClientID:  connID,
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
			c.holder.clearAll()
		}
	})
	return err
}
