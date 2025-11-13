package realtimeapi

import (
	"encoding/json"
	"strings"
	"sync"

	types "smart-speaker/internal/types"
)

type ToolRouter struct {
	mu     sync.Mutex
	states map[string]*toolCallState
	queue  []types.ToolRequest
}

type toolCallState struct {
	ResponseID string
	ToolCallID string
	Name       string
	Buffer     strings.Builder
}

func NewToolRouter() *ToolRouter {
	return &ToolRouter{states: make(map[string]*toolCallState)}
}

func (r *ToolRouter) Handle(msg wsMessage) bool {
	switch msg["type"].(string) {
	case "response.tool_calls.created", "response.tool_call.created":
		r.recordToolCalls(msg)
		return true
	case "response.tool_calls.delta", "response.tool_call.delta":
		r.appendToolDelta(msg)
		return true
	case "response.tool_calls.completed", "response.tool_call.completed":
		r.completeToolCalls(msg)
		return true
	case "response.function_call_arguments.delta":
		r.handleFunctionCallDelta(msg)
		return true
	case "response.function_call_arguments.done":
		r.handleFunctionCallDone(msg)
		return true
	default:
		return false
	}
}

func (r *ToolRouter) PopEvent() (types.Event, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.queue) == 0 {
		return types.Event{}, false
	}
	req := r.queue[0]
	r.queue = r.queue[1:]
	return types.Event{Kind: types.EventToolRequest, Payload: req}, true
}

func (r *ToolRouter) handleFunctionCallDelta(msg wsMessage) {
	responseID := asString(msg["response_id"])
	callID := asString(msg["call_id"])
	if callID == "" {
		return
	}
	state := r.ensureToolState(responseID, callID)
	if name := asString(msg["name"]); name != "" {
		state.Name = name
	}
	if delta := asString(msg["delta"]); delta != "" {
		state.Buffer.WriteString(delta)
	}
}

func (r *ToolRouter) handleFunctionCallDone(msg wsMessage) {
	responseID := asString(msg["response_id"])
	callID := asString(msg["call_id"])
	if callID == "" {
		return
	}
	state := r.popToolState(callID)
	if state == nil {
		state = &toolCallState{ResponseID: responseID, ToolCallID: callID}
	}
	if state.ResponseID == "" {
		state.ResponseID = responseID
	}
	if name := asString(msg["name"]); name != "" {
		state.Name = name
	}
	if args := asString(msg["arguments"]); args != "" {
		state.Buffer.Reset()
		state.Buffer.WriteString(args)
	}
	r.emitToolRequest(state)
}

func (r *ToolRouter) recordToolCalls(msg wsMessage) {
	responseID := asString(msg["response_id"])
	if arr, ok := msg["tool_calls"].([]any); ok {
		for _, entry := range arr {
			if call, ok := entry.(map[string]any); ok {
				r.updateToolState(responseID, call)
			}
		}
	}
}

func (r *ToolRouter) appendToolDelta(msg wsMessage) {
	responseID := asString(msg["response_id"])
	if callID := asString(msg["tool_call_id"]); callID != "" {
		state := r.ensureToolState(responseID, callID)
		if delta, ok := msg["delta"].(map[string]any); ok {
			r.applyFunctionDelta(state, delta)
		}
		return
	}
	if arr, ok := msg["tool_calls"].([]any); ok {
		for _, entry := range arr {
			if call, ok := entry.(map[string]any); ok {
				r.updateToolState(responseID, call)
			}
		}
	}
}

func (r *ToolRouter) completeToolCalls(msg wsMessage) {
	responseID := asString(msg["response_id"])
	var ids []string
	if arr, ok := msg["tool_call_ids"].([]any); ok {
		for _, id := range arr {
			ids = append(ids, asString(id))
		}
	}
	if len(ids) == 0 {
		if arr, ok := msg["tool_calls"].([]any); ok {
			for _, entry := range arr {
				if call, ok := entry.(map[string]any); ok {
					ids = append(ids, asString(call["id"]))
					r.updateToolState(responseID, call)
				}
			}
		}
	}
	for _, id := range ids {
		state := r.popToolState(id)
		if state == nil {
			continue
		}
		r.emitToolRequest(state)
	}
}

func (r *ToolRouter) updateToolState(responseID string, call map[string]any) {
	callID := asString(call["id"])
	if callID == "" {
		callID = asString(call["tool_call_id"])
	}
	if callID == "" {
		return
	}
	state := r.ensureToolState(responseID, callID)
	if fn, ok := call["function"].(map[string]any); ok {
		r.applyFunctionDelta(state, fn)
	}
	if delta, ok := call["delta"].(map[string]any); ok {
		r.applyFunctionDelta(state, delta)
	}
}

func (r *ToolRouter) applyFunctionDelta(state *toolCallState, fn map[string]any) {
	if name := asString(fn["name"]); name != "" {
		state.Name = name
	}
	if args := asString(fn["arguments"]); args != "" {
		state.Buffer.WriteString(args)
	}
}

func (r *ToolRouter) ensureToolState(responseID, callID string) *toolCallState {
	r.mu.Lock()
	defer r.mu.Unlock()
	if state, ok := r.states[callID]; ok {
		return state
	}
	state := &toolCallState{ResponseID: responseID, ToolCallID: callID}
	r.states[callID] = state
	return state
}

func (r *ToolRouter) popToolState(callID string) *toolCallState {
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.states[callID]
	delete(r.states, callID)
	return state
}

func (r *ToolRouter) emitToolRequest(state *toolCallState) {
	argsJSON := strings.TrimSpace(state.Buffer.String())
	if argsJSON == "" {
		argsJSON = "{}"
	}
	req := types.ToolRequest{
		ResponseID: state.ResponseID,
		ToolCallID: state.ToolCallID,
		Name:       state.Name,
		Arguments:  json.RawMessage([]byte(argsJSON)),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.queue = append(r.queue, req)
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
