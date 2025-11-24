package receiver

import (
	types "smart-speaker/internal/types"
)

type audioMessageHandler struct {
	tracker *responseTracker
}

func newAudioMessageHandler(tracker *responseTracker) *audioMessageHandler {
	return &audioMessageHandler{tracker: tracker}
}

func (h *audioMessageHandler) Handle(msg wsMessage) []types.Event {
	msgType := asString(msg["type"])
	switch msgType {
	case "response.output_audio.delta", "response.audio.delta":
		delta := asString(msg["delta"])
		if delta == "" {
			return nil
		}
		return []types.Event{{Kind: types.EventRealtimeAudio, Payload: types.OutputAudio{Role: "assistant", Audio: delta}}}
	case "response.delta", "response.done":
		return h.handleResponseMessage(msgType, msg)
	}
	return nil
}

func (h *audioMessageHandler) handleResponseMessage(msgType string, msg wsMessage) []types.Event {
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
		audioEvents, assistant := collectAudioEvents(role, content)
		if assistant {
			hasAssistant = true
		}
		events = append(events, audioEvents...)
	}
	if len(events) == 0 {
		return nil
	}
	if msgType == "response.delta" && hasAssistant {
		h.tracker.markDelta(respID)
	}
	return events
}

func collectAudioEvents(role string, content []any) ([]types.Event, bool) {
	var events []types.Event
	assistantOutput := false
	for _, part := range content {
		partMap, ok := part.(map[string]any)
		if !ok {
			continue
		}
		switch asString(partMap["type"]) {
		case "output_audio", "audio":
			audioMap, ok := partMap["audio"].(map[string]any)
			if !ok {
				continue
			}
			data, ok := audioMap["data"].([]any)
			if !ok {
				continue
			}
			actualRole := role
			if actualRole == "" {
				actualRole = "assistant"
			}
			for _, chunk := range data {
				audio := asString(chunk)
				if audio == "" {
					continue
				}
				events = append(events, types.Event{Kind: types.EventRealtimeAudio, Payload: types.OutputAudio{Role: actualRole, Audio: audio}})
				if actualRole == "assistant" {
					assistantOutput = true
				}
			}
		}
	}
	return events, assistantOutput
}
