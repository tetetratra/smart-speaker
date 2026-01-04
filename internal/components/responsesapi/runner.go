package responsesapi

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"smart-speaker/internal/graph"
	"smart-speaker/internal/state"
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
	lastResponseID string
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
		systemPrompt:   strings.TrimSpace(cfg.Instructions),
	}
	r.loadResponseIDFromFile()
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
				r.handleRequest(strings.TrimSpace(req.Role), text)
			case types.EventTextInput:
				line, ok := evt.Payload.(types.OutputLine)
				if !ok {
					continue
				}
				text := strings.TrimSpace(line.Text)
				if text == "" {
					continue
				}
				r.handleRequest("user", text)
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

func (r *runner) handleRequest(role, text string) {
	prevID := r.currentResponseID()
	systemPrompt := r.systemPromptIfNeeded(prevID)
	resp, err := r.client.CreateResponse(r.ctx, role, appendOutputConstraint(text), prevID, systemPrompt)
	if err != nil {
		if prevID != "" && isInvalidPreviousResponseID(err) {
			r.clearResponseID()
			systemPrompt = r.systemPromptIfNeeded("")
			resp, err = r.client.CreateResponse(r.ctx, role, appendOutputConstraint(text), "", systemPrompt)
			if err == nil {
				r.handleResponsesResponse(resp)
				return
			}
		}
		log.Printf("responsesapi: request error: %v", err)
		return
	}
	r.handleResponsesResponse(resp)
}

func (r *runner) systemPromptIfNeeded(previousResponseID string) string {
	if strings.TrimSpace(previousResponseID) != "" {
		return ""
	}
	return r.systemPrompt
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
	const suffix = "（記号・URLの使用禁止）"
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return trimmed
	}
	return trimmed + " " + suffix
}

func (r *runner) handleResponsesResponse(resp types.ResponsesResponse) {
	r.setCurrentResponseID(resp.ResponseID)
	r.emit(types.Event{Kind: types.EventResponsesResponse, Payload: resp})
	if len(resp.ToolCalls) > 0 {
		for _, call := range resp.ToolCalls {
			r.mu.Lock()
			r.toolResponseID[call.ToolCallID] = resp.ResponseID
			r.mu.Unlock()
			r.emit(types.Event{Kind: types.EventToolRequest, Payload: call})
		}
	}
	if len(resp.MCPCalls) > 0 {
		for _, call := range resp.MCPCalls {
			r.emit(types.Event{Kind: types.EventMCPCall, Payload: call})
		}
	}
	if !resp.HasResponse {
		return
	}
	state.SetLastActivityAt(time.Now())
	state.SetLastAssistantTalkAt(time.Now())
	r.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "assistant", Text: resp.Text, ResponseID: resp.ResponseID, Source: "responses"}})
	r.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "assistant", ResponseID: resp.ResponseID, Final: true, Source: "responses"}})
}

func (r *runner) currentResponseID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastResponseID
}

func (r *runner) setCurrentResponseID(responseID string) {
	if strings.TrimSpace(responseID) == "" {
		return
	}
	r.mu.Lock()
	r.lastResponseID = responseID
	r.mu.Unlock()
	r.persistResponseID(responseID)
}

func (r *runner) emit(evt types.Event) {
	select {
	case <-r.ctx.Done():
		return
	case r.downstream <- evt:
	}
}

func (r *runner) loadResponseIDFromFile() {
	data, err := os.ReadFile(responseIDFilePath())
	if err != nil {
		return
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return
	}
	r.lastResponseID = id
}

func (r *runner) persistResponseID(responseID string) {
	path := responseIDFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("responsesapi: failed to create tmp dir: %v", err)
		return
	}
	if err := os.WriteFile(path, []byte(responseID+"\n"), 0o644); err != nil {
		log.Printf("responsesapi: failed to write response id: %v", err)
	}
}

func (r *runner) clearResponseID() {
	r.mu.Lock()
	r.lastResponseID = ""
	r.mu.Unlock()
	if err := os.Remove(responseIDFilePath()); err != nil && !os.IsNotExist(err) {
		log.Printf("responsesapi: failed to remove response id: %v", err)
	}
}

func responseIDFilePath() string {
	return filepath.Join("tmp", "response_id.txt")
}

func isInvalidPreviousResponseID(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "previous_response_id") || strings.Contains(msg, "response_id")
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
