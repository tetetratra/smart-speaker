package receiver

import (
	types "smart-speaker/internal/types"
)

type textMessageHandler struct{}

func newTextMessageHandler() *textMessageHandler {
	return &textMessageHandler{}
}

func (h *textMessageHandler) Handle(msg wsMessage) []types.Event {
	msgType := asString(msg["type"])
	switch msgType {
	case "conversation.item.input_audio_transcription.delta", "conversation.item.input_audio_transcription.completed":
		transcript := asString(msg["transcript"])
		if transcript == "" {
			return nil
		}
		events := []types.Event{{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "user", Text: transcript, Source: "realtime"}}}
		if msgType == "conversation.item.input_audio_transcription.completed" {
			events = append(events, types.Event{Kind: types.EventTranscriptFinal, Payload: types.TranscriptEvent{Text: transcript, Final: true}})
		} else {
			events = append(events, types.Event{Kind: types.EventTranscriptPartial, Payload: types.TranscriptEvent{Text: transcript}})
		}
		return events
	case "error", "response.error":
		if detail, ok := msg["error"].(map[string]any); ok {
			if message, ok := detail["message"].(string); ok {
				return []types.Event{{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "error", Text: message}}}
			}
		}
	}
	return nil
}
