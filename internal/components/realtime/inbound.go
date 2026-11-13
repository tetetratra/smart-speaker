package realtime

import (
	"context"
	"github.com/tetetratra/smart-speaker/internal/types"
	"time"
)

func (s *stage) queueInbound(input types.ConversationInput) {
	s.inbound = append(s.inbound, input)
}

func (s *stage) startInbound(ctx context.Context) bool {
	if len(s.inbound) == 0 || s.speaking || s.active != "" || s.wantResponse || s.connecting || len(s.repairs) > 0 || s.hasPendingOutput() {
		return false
	}
	if !s.ready || s.transport == nil {
		s.connect(ctx)
		return false
	}
	input := s.inbound[0]
	s.inbound = s.inbound[1:]
	record := input.Record
	if input.External || record.GenerationID == 0 {
		change, _ := s.controller.Start(record.ID)
		s.emit(ctx, types.Event{Kind: types.EventGenerationChanged, Payload: change})
		record.GenerationID = change.GenerationID
	}

	s.saveRecord(ctx, record)
	s.lastActivity = time.Now()
	s.addContext(input)
	s.emit(ctx, types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: record.Role, Text: record.Text, Source: record.Source, GenerationID: record.GenerationID, ResponseID: record.ID, Final: true}})
	s.wantResponse = true
	s.maybeRespond(ctx)
	return true
}
