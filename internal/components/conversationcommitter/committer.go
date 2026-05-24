package conversationcommitter

import (
	"context"
	"log"

	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type committer struct {
	history    *conversationhistory.Store
	generation *generation.Store
	emit       func(types.Event)
}

func (c *committer) Commit(ctx context.Context, req types.ConversationCommitRequest) {
	if c.history == nil {
		log.Printf("conversationcommitter: history store is nil")
		return
	}
	current := types.GenerationID(0)
	if c.generation != nil {
		current = c.generation.Current()
	}
	record := conversationhistory.NewRecord(req, current)
	if record.Text == "" {
		return
	}
	c.history.Append(record)

	switch record.Role {
	case types.RoleUser:
		c.emitUser(ctx, record)
	case types.RoleAgent:
		c.emitAgent(ctx, record)
	case types.RoleToolCall:
		c.emitToolCall(ctx, record)
	case types.RoleToolResult:
		c.emitToolResult(ctx, record)
	}
}

func (c *committer) emitUser(ctx context.Context, record types.ConversationRecord) {
	c.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{
		Role:         types.RoleUser,
		Text:         record.Text,
		Source:       record.Source,
		GenerationID: record.GenerationID,
		Final:        true,
	}})
	c.emit(types.Event{Kind: types.EventLLMRequest, Payload: types.LLMRequest{
		RequestID:    record.ID,
		Role:         types.RoleUser,
		Text:         record.Text,
		GenerationID: record.GenerationID,
	}})
	_ = ctx
}

func (c *committer) emitAgent(ctx context.Context, record types.ConversationRecord) {
	c.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{
		Role:         types.RoleAgent,
		Text:         record.Text,
		Source:       record.Source,
		GenerationID: record.GenerationID,
		Final:        true,
	}})
	_ = ctx
}

func (c *committer) emitToolCall(ctx context.Context, record types.ConversationRecord) {
	c.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{
		Role:         types.RoleToolCall,
		Text:         record.Text,
		Source:       record.Source,
		GenerationID: record.GenerationID,
		Final:        true,
	}})
	_ = ctx
}

func (c *committer) emitToolResult(ctx context.Context, record types.ConversationRecord) {
	c.emit(types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{
		Role:         types.RoleToolResult,
		Text:         record.Text,
		Source:       record.Source,
		GenerationID: record.GenerationID,
		Final:        true,
	}})
	c.emit(types.Event{Kind: types.EventLLMRequest, Payload: types.LLMRequest{
		RequestID:    record.ID,
		Role:         types.RoleToolResult,
		Text:         record.Text,
		GenerationID: record.GenerationID,
	}})
	_ = ctx
}
