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

	mu             sync.Mutex
	toolResponseID map[string]string
}

// NewStage はResponses API呼び出しのステージを構築します。
func NewStage(cfg Config) (*graph.Stage, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	r := &runner{
		upstream:       make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream:     make(chan types.Event, graph.DefaultChannelBufferSize),
		client:         client,
		toolResponseID: map[string]string{},
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
			switch evt.Kind {
			case types.EventResponsesRequest:
				req, ok := evt.Payload.(types.ResponsesRequest)
				if !ok {
					continue
				}
				text := strings.TrimSpace(req.Text)
				if text == "" {
					continue
				}
				r.handleRequest(text)
			case types.EventTextInput:
				line, ok := evt.Payload.(types.OutputLine)
				if !ok {
					continue
				}
				if strings.TrimSpace(line.Role) == "system" {
					continue
				}
				text := strings.TrimSpace(line.Text)
				if text == "" {
					continue
				}
				r.handleRequest(text)
			case types.EventToolResponse:
				resp, ok := evt.Payload.(types.ToolResponse)
				if !ok {
					continue
				}
				r.handleToolResponse(resp)
			}
		}
	}
}

func (r *runner) handleRequest(text string) {
	resp, err := r.client.CreateResponse(r.ctx, appendOutputConstraint(text))
	if err != nil {
		log.Printf("responsesapi: request error: %v", err)
		return
	}
	r.handleResponsesResponse(resp)
}

func (r *runner) handleToolResponse(resp types.ToolResponse) {
	r.mu.Lock()
	responseID := r.toolResponseID[resp.ToolCallID]
	delete(r.toolResponseID, resp.ToolCallID)
	r.mu.Unlock()
	if responseID == "" {
		log.Printf("responsesapi: missing response id for tool call %s", resp.ToolCallID)
		return
	}
	out := strings.TrimSpace(string(resp.Output))
	next, err := r.client.SubmitToolOutput(r.ctx, responseID, resp.ToolCallID, appendOutputConstraint(out))
	if err != nil {
		log.Printf("responsesapi: tool output error: %v", err)
		return
	}
	r.handleResponsesResponse(next)
}

func appendOutputConstraint(text string) string {
	const suffix = "（マークダウン・記号・URLを使わず、1文程度で返答してください）"
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return trimmed
	}
	return trimmed + " " + suffix
}

func (r *runner) handleResponsesResponse(resp types.ResponsesResponse) {
	r.emit(types.Event{Kind: types.EventResponsesResponse, Payload: resp})
	if len(resp.ToolCalls) > 0 {
		for _, call := range resp.ToolCalls {
			r.mu.Lock()
			r.toolResponseID[call.ToolCallID] = resp.ResponseID
			r.mu.Unlock()
			r.emit(types.Event{Kind: types.EventToolRequest, Payload: call})
		}
	}
	if !resp.HasResponse {
		return
	}
	r.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "assistant", Text: resp.Text, ResponseID: resp.ResponseID, Source: "responses"}})
	r.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "assistant", ResponseID: resp.ResponseID, Final: true, Source: "responses"}})
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
