package receiver

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	types "smart-speaker/internal/types"
)

type wsMessage map[string]any

const (
	debugPrintMsgType  = true
	debugDumpResponses = false
)

type responseTracker struct {
	seen map[string]bool
}

func newResponseTracker() *responseTracker {
	return &responseTracker{seen: make(map[string]bool)}
}

func (t *responseTracker) markDelta(respID string) {
	if respID == "" {
		return
	}
	t.seen[respID] = true
}

func (t *responseTracker) shouldSkipDone(respID string) bool {
	if respID == "" {
		return false
	}
	if t.seen[respID] {
		delete(t.seen, respID)
		return true
	}
	return false
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

type reader interface {
	Read(context.Context) ([]byte, error)
}

type messageHandler interface {
	Handle(wsMessage) []types.Event
}

// Runner pulls messages from the realtime client and sends parsed events downstream.
type Runner struct {
	ctx        context.Context
	client     reader
	downstream chan<- types.Event
	handlers   []messageHandler
	buffer     []types.Event
}

func NewRunner(ctx context.Context, client reader, downstream chan<- types.Event) *Runner {
	tracker := newResponseTracker()
	return &Runner{
		ctx:        ctx,
		client:     client,
		downstream: downstream,
		handlers: []messageHandler{
			newToolMessageHandler(),
			newAudioMessageHandler(tracker),
			newTextMessageHandler(tracker),
		},
	}
}

func (r *Runner) Run() {
	defer close(r.downstream)
	for {
		if len(r.buffer) == 0 {
			if err := r.readNextMessage(); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				log.Printf("realtime read error: %v", err)
				return
			}
			continue
		}
		evt := r.buffer[0]
		r.buffer = r.buffer[1:]
		select {
		case <-r.ctx.Done():
			return
		case r.downstream <- evt:
		}
	}
}

func (r *Runner) readNextMessage() error {
	if err := r.ctx.Err(); err != nil {
		return err
	}
	data, err := r.client.Read(r.ctx)
	if err != nil {
		return err
	}
	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		log.Printf("unmarshal error: %v", err)
		return nil
	}
	if debugDumpResponses {
		if dump, err := json.MarshalIndent(msg, "", "  "); err == nil {
			log.Println(string(dump))
		}
	}
	if debugPrintMsgType {
		if msgType := asString(msg["type"]); msgType != "" {
			log.Println(msgType)
		}
	}
	for _, handler := range r.handlers {
		if events := handler.Handle(msg); len(events) > 0 {
			r.buffer = append(r.buffer, events...)
		}
	}
	return nil
}
