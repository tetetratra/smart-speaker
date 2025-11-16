package sender

import (
	"context"
	"errors"
	"log"
	"strings"

	types "smart-speaker/internal/types"
)

// Client defines the methods required by the sender runner.
type Client interface {
	Process(context.Context, types.AudioChunk) error
	Send(any) error
}

// Runner pulls events from the upstream channel and forwards them to the Realtime API.
type Runner struct {
	ctx      context.Context
	client   Client
	upstream <-chan types.Event
}

func NewRunner(ctx context.Context, client Client, upstream <-chan types.Event) *Runner {
	return &Runner{ctx: ctx, client: client, upstream: upstream}
}

func (r *Runner) Run() {
	for {
		select {
		case <-r.ctx.Done():
			return
		case evt, ok := <-r.upstream:
			if !ok {
				return
			}
			r.handleEvent(evt)
		}
	}
}

func (r *Runner) handleEvent(evt types.Event) {
	switch evt.Kind {
	case types.EventAudioChunk:
		chunk, ok := evt.Payload.(types.AudioChunk)
		if !ok {
			log.Printf("realtime sender: unexpected audio payload type %T", evt.Payload)
			return
		}
		if err := r.client.Process(r.ctx, chunk); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("realtime send error: %v", err)
		}
	case types.EventToolResponse:
		resp, ok := evt.Payload.(types.ToolResponse)
		if !ok {
			log.Printf("realtime sender: unexpected tool response payload %T", evt.Payload)
			return
		}
		if err := r.sendToolResponse(resp); err != nil {
			log.Printf("realtime tool response error: %v", err)
		}
	case types.EventTextInput:
		line, ok := evt.Payload.(types.OutputLine)
		if !ok {
			log.Printf("realtime sender: unexpected text payload type %T", evt.Payload)
			return
		}
		if err := r.sendTextInput(line); err != nil {
			log.Printf("realtime text input error: %v", err)
		}
	}
}

func (r *Runner) sendToolResponse(resp types.ToolResponse) error {
	toolOutput := wsMessage{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type":    "function_call_output",
			"call_id": resp.ToolCallID,
			"output":  string(resp.Output),
		},
	}
	if err := r.client.Send(toolOutput); err != nil {
		return err
	}
	return r.client.Send(wsMessage{
		"type": "response.create",
		"response": map[string]any{
			"modalities":   []string{"text"},
			"instructions": "Use the latest tool output to continue responding in Japanese.",
		},
	})
}

func (r *Runner) sendTextInput(line types.OutputLine) error {
	text := strings.TrimSpace(line.Text)
	if text == "" {
		return nil
	}
	role := line.Role
	if role == "" {
		role = "user"
	}
	msg := wsMessage{
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
	if err := r.client.Send(msg); err != nil {
		return err
	}
	return r.client.Send(wsMessage{
		"type": "response.create",
		"response": map[string]any{
			"modalities": []string{"text"},
		},
	})
}

type wsMessage map[string]any
