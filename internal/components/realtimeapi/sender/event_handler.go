package sender

import (
	"context"
	"errors"
	"log"

	types "smart-speaker/internal/types"
)

// EventHandler processes upstream events and dispatches them via the client.
type EventHandler struct {
	ctx    context.Context
	client Client
}

func NewEventHandler(ctx context.Context, client Client, voice string) *EventHandler {
	return &EventHandler{ctx: ctx, client: client}
}

func (h *EventHandler) Handle(evt types.Event) {
	switch evt.Kind {
	case types.EventAudioChunk:
		h.handleAudioChunk(evt.Payload)
	}
}

func (h *EventHandler) handleAudioChunk(payload any) {
	chunk, ok := payload.(types.AudioChunk)
	if !ok {
		log.Printf("realtime sender: unexpected audio payload type %T", payload)
		return
	}
	msg := map[string]any{
		"type":  "input_audio_buffer.append",
		"audio": string(chunk),
	}
	log.Printf("realtime send append len=%d", len(chunk))
	if err := h.client.Send(msg); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		log.Printf("realtime send error: %v", err)
	}
}
