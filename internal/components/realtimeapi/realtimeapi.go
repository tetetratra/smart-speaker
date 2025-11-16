package realtimeapi

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"

	"smart-speaker/internal/components/realtimeapi/receiver"
	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

type realtimeAPI struct {
	client     *Client
	stream     *receiver.EventStream
	upstream   chan types.Event
	downstream chan types.Event
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once
	lineWG     sync.WaitGroup
}

// NewStage constructs a realtime stage with the given config.
func NewStage(ctx context.Context, cfg Config) (*graph.Stage, error) {
	stageCtx, cancel := context.WithCancel(ctx)
	client, err := NewClient(stageCtx, cfg)
	if err != nil {
		cancel()
		return nil, err
	}
	s := &realtimeAPI{
		client:     client,
		stream:     receiver.NewEventStream(client),
		upstream:   make(chan types.Event),
		downstream: make(chan types.Event),
		ctx:        stageCtx,
		cancel:     cancel,
	}
	return &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.Close,
	}, nil
}

func (s *realtimeAPI) run(context.Context) {
	s.lineWG.Add(2)
	go func() {
		defer s.lineWG.Done()
		s.runSender()
	}()
	go func() {
		defer s.lineWG.Done()
		s.runReceiver()
	}()
}

func (s *realtimeAPI) runSender() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case evt, ok := <-s.upstream:
			if !ok {
				return
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

func (s *realtimeAPI) runReceiver() {
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

// Close closes the underlying client and owned channels.
func (s *realtimeAPI) Close() error {
	var err error
	s.once.Do(func() {
		s.cancel()
		close(s.upstream)
		s.lineWG.Wait()
		err = s.client.Close()
		log.Println("realtimeapi: stage closed")
	})
	return err
}

func (s *realtimeAPI) sendToolResponse(resp types.ToolResponse) error {
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

func (s *realtimeAPI) sendTextInput(line types.OutputLine) error {
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
