package realtime

import (
	"context"

	"github.com/tetetratra/smart-speaker/internal/types"
)

// recordDirectToolCall は、予約ジョブがAIを通さずに呼んだツールを会話履歴に残す。
// 応答は作らない。接続中のセッションには、次の会話で参照できるよう実行結果を文脈として足す。
func (s *stage) recordDirectToolCall(ctx context.Context, call types.DirectToolCall) {
	gen := s.cfg.Generation.Current()
	id := call.Request.ToolCallID
	s.record(ctx, "call/"+id, types.RoleToolCall, call.Request.Name+": "+string(call.Request.Arguments), gen, call.Source)
	s.record(ctx, "result/"+id, types.RoleToolResult, string(call.Result.Output), gen, call.Source)
	switch {
	case s.transport == nil:
		// 未接続なら、次に接続したとき会話履歴から文脈が復元される。
	case s.ready:
		s.sendResultContext(call.Result)
	default:
		s.contextResults = append(s.contextResults, call.Result)
	}
}
