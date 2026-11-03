package generationcontroller

import (
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// Controller はAPI受信順に呼ばれ、世代更新と再生停止通知の生成を一か所に集める。
// 応答開始より先に更新を確定できるよう、非同期の出力処理とは分離する。
type Controller struct {
	store    *generation.Store
	lastItem string
}

func New(store *generation.Store) *Controller { return &Controller{store: store} }
func (c *Controller) Start(itemID string) (types.GenerationChanged, bool) {
	if itemID != "" && itemID == c.lastItem {
		return types.GenerationChanged{}, false
	}
	c.lastItem = itemID
	return types.GenerationChanged{GenerationID: c.store.Next(), ItemID: itemID}, true
}
