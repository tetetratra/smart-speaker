package receiver

import (
	"encoding/json"
	"strings"

	types "smart-speaker/internal/types"
)

type toolMessageHandler struct {
	states map[string]*toolCallState
}

type toolCallState struct {
	ResponseID string
	ToolCallID string
	Name       string
	Buffer     strings.Builder
}

func newToolMessageHandler() *toolMessageHandler {
	return &toolMessageHandler{states: make(map[string]*toolCallState)}
}

func (h *toolMessageHandler) Handle(msg wsMessage) []types.Event {
	msgType := asString(msg["type"])
	switch msgType {
	case "response.tool_calls.created", "response.tool_call.created":
		h.recordToolCalls(msg)
	case "response.tool_calls.delta", "response.tool_call.delta":
		h.appendToolDelta(msg)
	case "response.tool_calls.completed", "response.tool_call.completed":
		return h.completeToolCalls(msg)
	case "response.function_call_arguments.delta":
		h.handleFunctionCallDelta(msg)
	case "response.function_call_arguments.done":
		return h.handleFunctionCallDone(msg)
	}
	return nil
}

func (h *toolMessageHandler) handleFunctionCallDelta(msg wsMessage) {
	responseID := asString(msg["response_id"])
	callID := asString(msg["call_id"])
	if callID == "" {
		return
	}
	state := h.ensureToolState(responseID, callID)
	if name := asString(msg["name"]); name != "" {
		state.Name = name
	}
	if delta := asString(msg["delta"]); delta != "" {
		state.Buffer.WriteString(delta)
	}
}

func (h *toolMessageHandler) handleFunctionCallDone(msg wsMessage) []types.Event {
	responseID := asString(msg["response_id"])
	callID := asString(msg["call_id"])
	if callID == "" {
		return nil
	}
	state := h.popToolState(callID)
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
	return []types.Event{h.buildToolEvent(state)}
}

func (h *toolMessageHandler) recordToolCalls(msg wsMessage) {
	responseID := asString(msg["response_id"])
	if arr, ok := msg["tool_calls"].([]any); ok {
		for _, entry := range arr {
			if call, ok := entry.(map[string]any); ok {
				h.updateToolState(responseID, call)
			}
		}
	}
}

func (h *toolMessageHandler) appendToolDelta(msg wsMessage) {
	responseID := asString(msg["response_id"])
	if callID := asString(msg["tool_call_id"]); callID != "" {
		state := h.ensureToolState(responseID, callID)
		if delta, ok := msg["delta"].(map[string]any); ok {
			h.applyFunctionDelta(state, delta)
		}
		return
	}
	if arr, ok := msg["tool_calls"].([]any); ok {
		for _, entry := range arr {
			if call, ok := entry.(map[string]any); ok {
				h.updateToolState(responseID, call)
			}
		}
	}
}

func (h *toolMessageHandler) completeToolCalls(msg wsMessage) []types.Event {
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
					h.updateToolState(responseID, call)
				}
			}
		}
	}
	var events []types.Event
	for _, id := range ids {
		state := h.popToolState(id)
		if state == nil {
			continue
		}
		events = append(events, h.buildToolEvent(state))
	}
	return events
}

func (h *toolMessageHandler) updateToolState(responseID string, call map[string]any) {
	callID := asString(call["id"])
	if callID == "" {
		callID = asString(call["tool_call_id"])
	}
	if callID == "" {
		return
	}
	state := h.ensureToolState(responseID, callID)
	if fn, ok := call["function"].(map[string]any); ok {
		h.applyFunctionDelta(state, fn)
	}
	if delta, ok := call["delta"].(map[string]any); ok {
		h.applyFunctionDelta(state, delta)
	}
}

func (h *toolMessageHandler) applyFunctionDelta(state *toolCallState, fn map[string]any) {
	if name := asString(fn["name"]); name != "" {
		state.Name = name
	}
	if args := asString(fn["arguments"]); args != "" {
		state.Buffer.WriteString(args)
	}
}

func (h *toolMessageHandler) ensureToolState(responseID, callID string) *toolCallState {
	if state, ok := h.states[callID]; ok {
		return state
	}
	state := &toolCallState{ResponseID: responseID, ToolCallID: callID}
	h.states[callID] = state
	return state
}

func (h *toolMessageHandler) popToolState(callID string) *toolCallState {
	state := h.states[callID]
	delete(h.states, callID)
	return state
}

func (h *toolMessageHandler) buildToolEvent(state *toolCallState) types.Event {
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
	return types.Event{Kind: types.EventToolRequest, Payload: req}
}
