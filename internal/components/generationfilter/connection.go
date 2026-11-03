package generationfilter

import (
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// Attach は世代判定を処理本体から切り離し、受信直後の接続境界へ置く。
// 対象StageはReceiveで受信し、追加の待機を挟まず処理を開始する。
func Attach(stage *graph.Stage, store *generation.Store) {
	stage.LatestOnly = true
	stage.ReceiveFilter = func(e types.Event) bool {
		id, ok := types.EventGeneration(e)
		return !ok || store.IsCurrent(types.GenerationID(id))
	}
}
