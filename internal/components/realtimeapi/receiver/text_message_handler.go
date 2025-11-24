package receiver

import (
	types "smart-speaker/internal/types"
)

type textMessageHandler struct {
	tracker *responseTracker
}

func newTextMessageHandler(tracker *responseTracker) *textMessageHandler {
	return &textMessageHandler{tracker: tracker}
}

func (h *textMessageHandler) Handle(msg wsMessage) []types.Event {
	msgType := asString(msg["type"])
	switch msgType {
	case "conversation.item.input_audio_transcription.delta", "conversation.item.input_audio_transcription.completed":
		transcript := asString(msg["transcript"])
		if transcript == "" {
			return nil
		}
		return []types.Event{{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "user", Text: transcript}}}
	case "response.output_text.delta":
		delta := asString(msg["delta"])
		if delta == "" {
			return nil
		}
		return []types.Event{{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "assistant", Text: delta}}}
	case "response.output_text":
		text := asString(msg["text"])
		if text == "" {
			return nil
		}
		return []types.Event{{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "assistant", Text: text}}}
	case "response.audio_transcript.delta", "response.audio_transcript.done":
		transcript := extractTranscript(msg)
		if transcript == "" {
			return nil
		}
		return []types.Event{{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "assistant", Text: transcript}}}
	case "response.delta", "response.done":
		return h.handleResponseMessage(msgType, msg)
	case "error", "response.error":
		if detail, ok := msg["error"].(map[string]any); ok {
			if message, ok := detail["message"].(string); ok {
				return []types.Event{{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "error", Text: message}}}
			}
		}
	}
	return nil
}

func (h *textMessageHandler) handleResponseMessage(msgType string, msg wsMessage) []types.Event {
	resp, ok := msg["response"].(map[string]any)
	if !ok {
		return nil
	}
	respID := asString(resp["id"])
	if msgType == "response.done" && h.tracker.shouldSkipDone(respID) {
		return nil
	}
	output, ok := resp["output"].([]any)
	if !ok {
		return nil
	}
	var events []types.Event
	hasAssistant := false
	for _, entry := range output {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		content, ok := item["content"].([]any)
		if !ok {
			continue
		}
		role := asString(item["role"])
		textEvents, assistant := collectTextEvents(role, content)
		if assistant {
			hasAssistant = true
		}
		events = append(events, textEvents...)
	}
	if len(events) == 0 {
		return nil
	}
	if msgType == "response.delta" && hasAssistant {
		h.tracker.markDelta(respID)
	}
	return events
}

func collectTextEvents(role string, content []any) ([]types.Event, bool) {
	var events []types.Event
	assistantOutput := false
	for _, part := range content {
		partMap, ok := part.(map[string]any)
		if !ok {
			continue
		}
		switch asString(partMap["type"]) {
		case "text":
			text := asString(partMap["text"])
			if text == "" {
				continue
			}
			actualRole := role
			if actualRole == "" {
				actualRole = "assistant"
			}
			events = append(events, types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: actualRole, Text: text}})
			if actualRole == "assistant" {
				assistantOutput = true
			}
		case "input_text":
			text := asString(partMap["text"])
			if text == "" {
				continue
			}
			actualRole := role
			if actualRole == "" {
				actualRole = "user"
			}
			events = append(events, types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: actualRole, Text: text}})
		}
	}
	return events, assistantOutput
}

func extractTranscript(msg wsMessage) string {
	if transcript := asString(msg["transcript"]); transcript != "" {
		return transcript
	}
	if delta, ok := msg["delta"].(map[string]any); ok {
		if transcript := asString(delta["transcript"]); transcript != "" {
			return transcript
		}
		if text := asString(delta["text"]); text != "" {
			return text
		}
	}
	return ""
}
