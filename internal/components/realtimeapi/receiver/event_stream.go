package receiver

import (
	"context"
	"encoding/json"
	"log"

	types "smart-speaker/internal/types"
)

type reader interface {
	Read(context.Context) ([]byte, error)
}

// EventStream pulls messages from Client, parses assistant lines,
// and routes tool call requests.
type EventStream struct {
	client reader
	parser *MessageParser
	router *ToolRouter
	buffer []types.Event
}

func NewEventStream(client reader) *EventStream {
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
			ev := s.buffer[0]
			s.buffer = s.buffer[1:]
			return ev, nil
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
		events := s.parser.Parse(msg)
		if len(events) == 0 {
			continue
		}
		s.buffer = append(s.buffer, events...)
	}
}
