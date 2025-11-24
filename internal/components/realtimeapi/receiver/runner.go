package receiver

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	types "smart-speaker/internal/types"
)

type wsMessage map[string]any

type RunnerOptions struct {
	DebugPrintMsgType  bool
	DebugDumpResponses bool
}

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
	ctx                context.Context
	client             reader
	downstream         chan<- types.Event
	handlers           []messageHandler
	buffer             []types.Event
	debugPrintMsgType  bool
	debugDumpResponses bool
}

func NewRunner(ctx context.Context, client reader, downstream chan<- types.Event, opts RunnerOptions) *Runner {
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
		debugPrintMsgType:  opts.DebugPrintMsgType,
		debugDumpResponses: opts.DebugDumpResponses,
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
	if r.debugPrintMsgType {
		if msgType := asString(msg["type"]); msgType != "" {
			log.Println(msgType)
		}
	}
	if r.debugDumpResponses {
		r.dumpMessage(msg)
	}
	for _, handler := range r.handlers {
		if events := handler.Handle(msg); len(events) > 0 {
			r.buffer = append(r.buffer, events...)
		}
	}
	return nil
}

func (r *Runner) dumpMessage(msg wsMessage) {
	pruned := truncateValue(msg)
	dump, err := json.MarshalIndent(pruned, "", "  ")
	if err != nil {
		log.Printf("marshal dump error: %v", err)
		return
	}
	log.Println(string(dump))
}

func truncateValue(v any) any {
	switch val := v.(type) {
	case wsMessage:
		res := make(wsMessage, len(val))
		for k, item := range val {
			res[k] = truncateValue(item)
		}
		return res
	case string:
		if len(val) <= 80 {
			return val
		}
		return val[:80] + "..."
	case []any:
		res := make([]any, len(val))
		for i, item := range val {
			res[i] = truncateValue(item)
		}
		return res
	case map[string]any:
		res := make(map[string]any, len(val))
		for k, item := range val {
			res[k] = truncateValue(item)
		}
		return res
	default:
		return val
	}
}
