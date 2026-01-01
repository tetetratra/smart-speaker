package responsesapi

import (
	"context"
	"log"
	"strings"
	"sync"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

type runner struct {
	upstream   chan types.Event
	downstream chan types.Event
	client     *Client

	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
}

// NewStage はResponses API呼び出しのステージを構築します。
func NewStage(cfg Config) (*graph.Stage, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	r := &runner{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		client:     client,
	}
	return &graph.Stage{
		Upstream:   r.upstream,
		Downstream: r.downstream,
		Run:        r.run,
		CloseFn:    r.close,
	}, nil
}

func (r *runner) run(parent context.Context) {
	r.ctx, r.cancel = context.WithCancel(parent)
	go r.consume()
}

func (r *runner) consume() {
	defer close(r.downstream)
	for {
		select {
		case <-r.ctx.Done():
			return
		case evt, ok := <-r.upstream:
			if !ok {
				return
			}
			if evt.Kind != types.EventResponsesRequest {
				continue
			}
			req, ok := evt.Payload.(types.ResponsesRequest)
			if !ok {
				continue
			}
			text := strings.TrimSpace(req.Text)
			if text == "" {
				continue
			}
			r.handleRequest(text)
		}
	}
}

func (r *runner) handleRequest(text string) {
	resp, err := r.client.CreateResponse(r.ctx, text)
	if err != nil {
		log.Printf("responsesapi: request error: %v", err)
		return
	}
	r.emit(types.Event{Kind: types.EventResponsesResponse, Payload: resp})
	if !resp.HasResponse {
		return
	}
	r.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "assistant", Text: resp.Text, ResponseID: resp.ResponseID}})
	r.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "assistant", ResponseID: resp.ResponseID, Final: true}})
}

func (r *runner) emit(evt types.Event) {
	select {
	case <-r.ctx.Done():
		return
	case r.downstream <- evt:
	}
}

func (r *runner) close() error {
	r.once.Do(func() {
		if r.cancel != nil {
			r.cancel()
		}
		close(r.upstream)
	})
	return nil
}
