package realtimeapi

import (
	"context"
	"encoding/json"
	"log"

	types "smart-speaker/internal/types"
)

// EventStream pulls messages from Client, parses assistant lines,
// and routes tool call requests.
type EventStream struct {
	client *Client
	parser *MessageParser
	router *ToolRouter
	buffer []types.OutputLine
}

func NewEventStream(client *Client) *EventStream {
	return &EventStream{
		client: client,
		parser: NewMessageParser(),
		router: NewToolRouter(),
	}
}

func (s *EventStream) Next(ctx context.Context) (types.Event, error) {
	for {
		if evt, ok := s.router.PopEvent(); ok {
			return evt, nil
		}
		if len(s.buffer) > 0 {
			line := s.buffer[0]
			s.buffer = s.buffer[1:]
			return types.Event{Kind: types.EventRealtimeOutput, Payload: line}, nil
		}
		if err := ctx.Err(); err != nil {
			return types.Event{}, err
		}
		data, err := s.client.Read(ctx)
		if err != nil {
			return types.Event{}, err
		}
		var msg wsMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("unmarshal error: %v", err)
			continue
		}
		if s.router.Handle(msg) {
			continue
		}
		lines := s.parser.Parse(msg)
		if len(lines) == 0 {
			continue
		}
		s.buffer = append(s.buffer, lines...)
	}
}
