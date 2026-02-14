package responsesapi

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

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
	toolRequestID  map[string]string
	systemPrompt   string
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
		toolRequestID:  map[string]string{},
		systemPrompt:   strings.TrimSpace(cfg.Instructions),
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
				r.handleRequest(req)
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

func (r *runner) handleRequest(req types.ResponsesRequest) {
	messages := req.Messages
	if len(messages) == 0 {
		text := strings.TrimSpace(req.Text)
		if text == "" {
			return
		}
		role := strings.TrimSpace(req.Role)
		if role == "" {
			role = "user"
		}
		messages = []types.ChatMessage{{Role: role, Content: text}}
	}
	systemPrompt := r.systemPrompt
	if req.SystemPrompt != nil {
		systemPrompt = strings.TrimSpace(*req.SystemPrompt)
	}
	systemPrompt = appendCurrentTimestamp(systemPrompt)
	resp, err := r.client.CreateResponse(r.ctx, messages, systemPrompt, req.ToolChoice, req.Tools)
	if err != nil {
		log.Printf("responsesapi: request error: %v", err)
		return
	}
	resp.RequestID = req.RequestID
	r.handleResponsesResponse(resp)
}

func appendCurrentTimestamp(prompt string) string {
	now := time.Now()
	dateLine := "現在日時：" + now.Format("2006/01/02") + " (" + formatJapaneseWeekday(now.Weekday()) + ")"
	timeLine := "現在時刻：" + now.Format("15:04:05")
	if strings.TrimSpace(prompt) == "" {
		return dateLine + "\n" + timeLine
	}
	return strings.TrimRight(prompt, "\n") + "\n" + dateLine + "\n" + timeLine
}

func formatJapaneseWeekday(w time.Weekday) string {
	switch w {
	case time.Monday:
		return "月"
	case time.Tuesday:
		return "火"
	case time.Wednesday:
		return "水"
	case time.Thursday:
		return "木"
	case time.Friday:
		return "金"
	case time.Saturday:
		return "土"
	case time.Sunday:
		return "日"
	default:
		return ""
	}
}

func (r *runner) handleToolResponse(resp types.ToolResponse) {
	r.mu.Lock()
	responseID := r.toolResponseID[resp.ToolCallID]
	requestID := r.toolRequestID[resp.ToolCallID]
	delete(r.toolResponseID, resp.ToolCallID)
	delete(r.toolRequestID, resp.ToolCallID)
	r.mu.Unlock()
	if responseID == "" {
		log.Printf("responsesapi: missing response id for tool call %s", resp.ToolCallID)
		return
	}
	out := strings.TrimSpace(string(resp.Output))
	next, err := r.client.SubmitToolOutput(r.ctx, responseID, resp.ToolCallID, appendOutputConstraint("assistant", out), nil)
	if err != nil {
		log.Printf("responsesapi: tool output error: %v", err)
		return
	}
	next.RequestID = requestID
	r.handleResponsesResponse(next)
}

func appendOutputConstraint(role, text string) string {
	trimmed := strings.TrimSpace(text)
	return trimmed
}

func (r *runner) handleResponsesResponse(resp types.ResponsesResponse) {
	resp.HasResponse = strings.TrimSpace(resp.Text) != ""
	r.emit(types.Event{Kind: types.EventResponsesResponse, Payload: resp})
	if len(resp.ToolCalls) > 0 {
		for _, call := range resp.ToolCalls {
			r.mu.Lock()
			r.toolResponseID[call.ToolCallID] = resp.ResponseID
			r.toolRequestID[call.ToolCallID] = resp.RequestID
			r.mu.Unlock()
			r.emit(types.Event{Kind: types.EventToolRequest, Payload: call})
		}
	}
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
