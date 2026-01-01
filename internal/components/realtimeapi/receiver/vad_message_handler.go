package receiver

import (
	"strings"

	types "smart-speaker/internal/types"
)

type vadMessageHandler struct{}

func newVADMessageHandler() *vadMessageHandler {
	return &vadMessageHandler{}
}

func (h *vadMessageHandler) Handle(msg wsMessage) []types.Event {
	msgType := asString(msg["type"])
	if msgType == "" {
		return nil
	}
	normalized := strings.TrimPrefix(msgType, "server.")
	switch normalized {
	case "input_audio_buffer.speech_started":
		return []types.Event{{Kind: types.EventVadStart, Payload: types.VadEvent{Type: "start"}}}
	case "input_audio_buffer.speech_stopped":
		return []types.Event{{Kind: types.EventVadStop, Payload: types.VadEvent{Type: "stop"}}}
	default:
		return nil
	}
}
