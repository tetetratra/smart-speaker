package conversationcommitter

import (
	"context"

	"smart-speaker/internal/states/generation"
	types "smart-speaker/internal/types"
)

type ResultAPI struct {
	input      chan<- types.Event
	generation *generation.Store
}

func (a *ResultAPI) CommitToolResult(ctx context.Context, result types.ToolResultRecord) error {
	if a == nil || a.input == nil {
		return nil
	}
	if a.generation != nil {
		result.CurrentGenerationID = a.generation.Current()
		result.Stale = result.GenerationID != result.CurrentGenerationID
	}
	req := types.ConversationCommitRequest{
		Role:         types.RoleTool,
		GenerationID: result.GenerationID,
		Source:       result.Name,
		ToolResult:   &result,
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case a.input <- types.Event{Kind: types.EventConversationCommitRequest, Payload: req}:
		return nil
	}
}
