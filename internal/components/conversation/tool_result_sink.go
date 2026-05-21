package conversation

import (
	"context"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

// ToolResultSink は tool runtime から conversation へ結果を戻す同期APIの入口です。
type ToolResultSink struct {
	ch chan types.ToolResponse
}

func NewToolResultSink() *ToolResultSink {
	return &ToolResultSink{
		ch: make(chan types.ToolResponse, graph.DefaultChannelBufferSize),
	}
}

func (s *ToolResultSink) Commit(ctx context.Context, resp types.ToolResponse) {
	if s == nil {
		return
	}
	select {
	case s.ch <- resp:
	case <-ctx.Done():
	}
}

func (s *ToolResultSink) C() <-chan types.ToolResponse {
	if s == nil {
		return nil
	}
	return s.ch
}
