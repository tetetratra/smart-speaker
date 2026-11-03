package generationfilter

import (
	"context"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// Attach は世代判定を処理本体から切り離し、受信直後の接続境界へ置く。
// 対象StageはReceiveで受信し、追加の待機を挟まず処理を開始する。
func Attach(stage *graph.Stage, store *generation.Store) {
	stage.LatestOnly = true
	stage.ReceiveFilter = func(_ context.Context, e types.Event) bool {
		id, ok := types.EventGeneration(e)
		return !ok || store.IsCurrent(types.GenerationID(id))
	}
}

// AttachToolRequests は未開始の旧要求を実行せず、結果不明のまま待ち続けないよう中断結果を返す。
// ツール実行結果の保存は、音声出力の破棄とは分ける。
func AttachToolRequests(stage *graph.Stage, store *generation.Store) {
	stage.LatestOnly = false
	stage.ReceiveFilter = func(ctx context.Context, e types.Event) bool {
		req, ok := e.Payload.(types.ToolRequest)
		if !ok || store.IsCurrent(req.GenerationID) {
			return true
		}
		result := types.ToolResultRecord{ToolCallID: req.ToolCallID, Name: req.Name, GenerationID: req.GenerationID, Output: []byte(`{"error":"interrupted before execution","executed":false}`)}
		// Graphは下流を無制限キューへ転送するため、処理待ちをこの通知へ伝えない。
		select {
		case <-ctx.Done():
		case stage.Downstream <- types.Event{Kind: types.EventConversationCommitRequest, Payload: types.ConversationCommitRequest{Role: types.RoleToolResult, ToolResult: &result}}:
		}
		return false
	}
}
