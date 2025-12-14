package sender

import (
	"context"
	"errors"
	"log"
	"strings"

	types "smart-speaker/internal/types"
)

// EventHandler processes upstream events and dispatches them via the client.
type EventHandler struct {
	ctx    context.Context
	client Client
	voice  string
}

func NewEventHandler(ctx context.Context, client Client, voice string) *EventHandler {
	return &EventHandler{ctx: ctx, client: client, voice: voice}
}

func (h *EventHandler) Handle(evt types.Event) {
	switch evt.Kind {
	case types.EventAudioChunk:
		h.handleAudioChunk(evt.Payload)
	case types.EventToolResponse:
		h.handleToolResponse(evt.Payload)
	case types.EventTextInput:
		h.handleTextInput(evt.Payload)
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

func (h *EventHandler) handleToolResponse(payload any) {
	resp, ok := payload.(types.ToolResponse)
	if !ok {
		log.Printf("realtime sender: unexpected tool response payload %T", payload)
		return
	}
	if err := h.sendToolResponse(resp); err != nil {
		log.Printf("realtime tool response error: %v", err)
	}
}

func (h *EventHandler) handleTextInput(payload any) {
	line, ok := payload.(types.OutputLine)
	if !ok {
		log.Printf("realtime sender: unexpected text payload type %T", payload)
		return
	}
	if err := h.sendTextInput(line); err != nil {
		log.Printf("realtime text input error: %v", err)
	}
}

func (h *EventHandler) sendToolResponse(resp types.ToolResponse) error {
	toolOutput := map[string]any{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type":    "function_call_output",
			"call_id": resp.ToolCallID,
			"output":  string(resp.Output),
		},
	}
	if err := h.client.Send(toolOutput); err != nil {
		return err
	}
	return h.client.Send(map[string]any{
		"type": "response.create",
		"response": map[string]any{
			// "modalities":   []string{"text", "audio"},
			"modalities":   []string{"text"},
			"instructions": "Use the latest tool output to continue responding in Japanese.",
		},
	})
}

func (h *EventHandler) sendTextInput(line types.OutputLine) error {
	text := strings.TrimSpace(line.Text)
	if text == "" {
		return nil
	}
	role := line.Role
	if role == "" {
		role = "user"
	}
	msg := map[string]any{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type": "message",
			"role": role,
			"content": []any{
				map[string]any{
					"type": "input_text",
					"text": text,
				},
			},
		},
	}
	if err := h.client.Send(msg); err != nil {
		return err
	}
	response := map[string]any{
		"type": "response.create",
		"response": map[string]any{
			// "modalities": []string{"text", "audio"},
			"modalities": []string{"text"},
		},
	}
	if h.voice != "" {
		response["response"].(map[string]any)["voice"] = h.voice
	}
	return h.client.Send(response)
}
