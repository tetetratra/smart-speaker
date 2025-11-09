package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

type toolCallState struct {
	ResponseID string
	ToolCallID string
	Name       string
	Buffer     strings.Builder
}

func (api *RealtimeAPI) handleToolMessage(msg wsMessage) bool {
	msgType, _ := msg["type"].(string)
	switch msgType {
	case "response.tool_calls.created", "response.tool_call.created":
		api.recordToolCalls(msg)
		return true
	case "response.tool_calls.delta", "response.tool_call.delta":
		api.appendToolDelta(msg)
		return true
	case "response.tool_calls.completed", "response.tool_call.completed":
		api.completeToolCalls(msg)
		return true
	case "response.function_call_arguments.delta":
		api.handleFunctionCallDelta(msg)
		return true
	case "response.function_call_arguments.done":
		api.handleFunctionCallDone(msg)
		return true
	default:
		return false
	}
}

func (api *RealtimeAPI) handleFunctionCallDelta(msg wsMessage) {
	responseID := asString(msg["response_id"])
	callID := asString(msg["call_id"])
	if callID == "" {
		return
	}
	state := api.ensureToolState(responseID, callID)
	if name := asString(msg["name"]); name != "" {
		state.Name = name
	}
	if delta := asString(msg["delta"]); delta != "" {
		state.Buffer.WriteString(delta)
	}
}

func (api *RealtimeAPI) handleFunctionCallDone(msg wsMessage) {
	responseID := asString(msg["response_id"])
	callID := asString(msg["call_id"])
	if callID == "" {
		return
	}
	state := api.popToolState(callID)
	if state == nil {
		state = &toolCallState{
			ResponseID: responseID,
			ToolCallID: callID,
		}
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
	go api.executeTool(state)
}

func (api *RealtimeAPI) recordToolCalls(msg wsMessage) {
	responseID := asString(msg["response_id"])
	if arr, ok := msg["tool_calls"].([]any); ok {
		for _, entry := range arr {
			if call, ok := entry.(map[string]any); ok {
				api.updateToolState(responseID, call)
			}
		}
	}
}

func (api *RealtimeAPI) appendToolDelta(msg wsMessage) {
	responseID := asString(msg["response_id"])
	if callID := asString(msg["tool_call_id"]); callID != "" {
		state := api.ensureToolState(responseID, callID)
		if delta, ok := msg["delta"].(map[string]any); ok {
			api.applyFunctionDelta(state, delta)
		}
		return
	}

	if arr, ok := msg["tool_calls"].([]any); ok {
		for _, entry := range arr {
			if call, ok := entry.(map[string]any); ok {
				api.updateToolState(responseID, call)
			}
		}
	}
}

func (api *RealtimeAPI) completeToolCalls(msg wsMessage) {
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
					api.updateToolState(responseID, call)
				}
			}
		}
	}
	for _, id := range ids {
		state := api.popToolState(id)
		if state == nil {
			continue
		}
		go api.executeTool(state)
	}
}

func (api *RealtimeAPI) updateToolState(responseID string, call map[string]any) {
	callID := asString(call["id"])
	if callID == "" {
		callID = asString(call["tool_call_id"])
	}
	if callID == "" {
		return
	}
	state := api.ensureToolState(responseID, callID)
	if fn, ok := call["function"].(map[string]any); ok {
		api.applyFunctionDelta(state, fn)
	}
	if delta, ok := call["delta"].(map[string]any); ok {
		api.applyFunctionDelta(state, delta)
	}
}

func (api *RealtimeAPI) applyFunctionDelta(state *toolCallState, fn map[string]any) {
	if name := asString(fn["name"]); name != "" {
		state.Name = name
	}
	if args := asString(fn["arguments"]); args != "" {
		state.Buffer.WriteString(args)
	}
}

func (api *RealtimeAPI) ensureToolState(responseID, callID string) *toolCallState {
	api.toolMu.Lock()
	defer api.toolMu.Unlock()
	if state, ok := api.toolStates[callID]; ok {
		return state
	}
	state := &toolCallState{ResponseID: responseID, ToolCallID: callID}
	api.toolStates[callID] = state
	return state
}

func (api *RealtimeAPI) popToolState(callID string) *toolCallState {
	api.toolMu.Lock()
	defer api.toolMu.Unlock()
	state := api.toolStates[callID]
	delete(api.toolStates, callID)
	return state
}

func (api *RealtimeAPI) executeTool(state *toolCallState) {
	argsJSON := strings.TrimSpace(state.Buffer.String())
	if argsJSON == "" {
		argsJSON = "{}"
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &payload); err != nil {
		log.Printf("tool arg parse error: %v", err)
		payload = map[string]any{}
	}

	log.Printf("🔧 tool call: name=%s call_id=%s args=%s", state.Name, state.ToolCallID, argsJSON)

	var result map[string]any
	switch state.Name {
	case "get_current_time":
		result = runCurrentTimeTool(payload)
	case "get_weather":
		result = runWeatherTool(payload)
	default:
		result = map[string]any{
			"error": fmt.Sprintf("unknown function: %s", state.Name),
		}
	}

	outputBytes, err := json.Marshal(result)
	if err != nil {
		log.Printf("tool result marshal error: %v", err)
		outputBytes = []byte(`{"error":"result encoding failed"}`)
	}

	toolOutput := wsMessage{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type":    "function_call_output",
			"call_id": state.ToolCallID,
			"output":  string(outputBytes),
		},
	}

	if err := api.send(toolOutput); err != nil {
		log.Printf("failed to send tool output: %v", err)
		return
	}

	if err := api.send(wsMessage{
		"type": "response.create",
		"response": map[string]any{
			"modalities":   []string{"text"},
			"instructions": "Use the latest tool output to continue responding in Japanese.",
		},
	}); err != nil {
		log.Printf("failed to request response after tool output: %v", err)
	}
}

func runCurrentTimeTool(args map[string]any) map[string]any {
	if hasArguments(args) {
		return map[string]any{
			"error": "get_current_time は引数を受け付けません",
		}
	}
	now := time.Now()
	return map[string]any{
		"iso8601":  now.Format(time.RFC3339),
		"timezone": now.Location().String(),
	}
}

func runWeatherTool(args map[string]any) map[string]any {
	if hasArguments(args) {
		return map[string]any{
			"error": "get_weather は引数を受け付けません",
		}
	}
	time.Sleep(5 * time.Second)
	return map[string]any{
		"forecast":    "晴れ",
		"temperature": 23.5,
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func hasArguments(args map[string]any) bool {
	if len(args) == 0 {
		return false
	}
	for _, v := range args {
		if v == nil {
			continue
		}
		if str, ok := v.(string); ok {
			if strings.TrimSpace(str) == "" {
				continue
			}
		}
		return true
	}
	return false
}
