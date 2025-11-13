package realtimeapi

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

// Stage wraps the Realtime API client into the new graph.Stage interface.
type Stage struct {
	client     *Client
	stream     *EventStream
	upstream   chan interface{}
	downstream chan interface{}
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once
}

// NewStage constructs a realtime stage with the given config.
func NewStage(ctx context.Context, cfg Config) (*Stage, error) {
	stageCtx, cancel := context.WithCancel(ctx)
	client, err := NewClient(stageCtx, cfg)
	if err != nil {
		cancel()
		return nil, err
	}
	s := &Stage{
		client:     client,
		stream:     NewEventStream(client),
		upstream:   make(chan interface{}),
		downstream: make(chan interface{}),
		ctx:        stageCtx,
		cancel:     cancel,
	}
	go s.runSender()
	go s.runReceiver()
	return s, nil
}

func (s *Stage) runSender() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case data, ok := <-s.upstream:
			if !ok {
				return
			}
			evt, ok := data.(types.Event)
			if !ok {
				log.Printf("realtime sender: unexpected upstream type %T", data)
				continue
			}
			switch evt.Kind {
			case types.EventAudioChunk:
				chunk, ok := evt.Payload.(types.AudioChunk)
				if !ok {
					log.Printf("realtime sender: unexpected audio payload type %T", evt.Payload)
					continue
				}
				if err := s.client.Process(s.ctx, chunk); err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					log.Printf("realtime send error: %v", err)
					return
				}
			case types.EventToolResponse:
				resp, ok := evt.Payload.(types.ToolResponse)
				if !ok {
					log.Printf("realtime sender: unexpected tool response payload %T", evt.Payload)
					continue
				}
				if err := s.sendToolResponse(resp); err != nil {
					log.Printf("realtime tool response error: %v", err)
				}
			case types.EventTextInput:
				line, ok := evt.Payload.(types.OutputLine)
				if !ok {
					log.Printf("realtime sender: unexpected text payload type %T", evt.Payload)
					continue
				}
				if err := s.sendTextInput(line); err != nil {
					log.Printf("realtime text input error: %v", err)
				}
			}
		}
	}
}

func (s *Stage) runReceiver() {
	defer close(s.downstream)
	for {
		evt, err := s.stream.Next(s.ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("realtime read error: %v", err)
			return
		}
		select {
		case <-s.ctx.Done():
			return
		case s.downstream <- evt:
		}
	}
}

func (s *Stage) Upstream() chan<- interface{} { return s.upstream }

func (s *Stage) Downstream() <-chan interface{} { return s.downstream }

// Close closes the underlying client and owned channels.
func (s *Stage) Close() error {
	var err error
	s.once.Do(func() {
		s.cancel()
		close(s.upstream)
		err = s.client.Close()
	})
	return err
}

func (s *Stage) sendToolResponse(resp types.ToolResponse) error {
	toolOutput := wsMessage{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type":    "function_call_output",
			"call_id": resp.ToolCallID,
			"output":  string(resp.Output),
		},
	}
	if err := s.client.Send(toolOutput); err != nil {
		return err
	}
	return s.client.Send(wsMessage{
		"type": "response.create",
		"response": map[string]any{
			"modalities":   []string{"text"},
			"instructions": "Use the latest tool output to continue responding in Japanese.",
		},
	})
}

func (s *Stage) sendTextInput(line types.OutputLine) error {
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
	if err := s.client.Send(msg); err != nil {
		return err
	}
	return s.client.Send(wsMessage{
		"type": "response.create",
		"response": map[string]any{
			"modalities": []string{"text"},
		},
	})
}

var _ graph.Stage = (*Stage)(nil)
