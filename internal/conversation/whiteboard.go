package conversation

import (
	"encoding/json"

	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// WhiteboardPolicy は情報取得後の表示更新を追跡する。ツール実体には会話制御を持ち込まない。
type WhiteboardPolicy struct {
	enabled   bool
	modes     map[string]string
	revision  map[types.GenerationID]uint64
	written   map[types.GenerationID]uint64
	requested map[types.GenerationID]uint64
}

func NewWhiteboardPolicy(modes map[string]string, enabled bool) *WhiteboardPolicy {
	return &WhiteboardPolicy{enabled: enabled, requested: map[types.GenerationID]uint64{}, modes: modes, revision: map[types.GenerationID]uint64{}, written: map[types.GenerationID]uint64{}}
}
func (p *WhiteboardPolicy) Revision(generation types.GenerationID) uint64 {
	if p == nil {
		return 0
	}
	return p.revision[generation]
}
func (p *WhiteboardPolicy) Pending(generation types.GenerationID) bool {
	return p != nil && p.enabled && p.revision[generation] > p.written[generation] && p.revision[generation] > p.requested[generation]
}
func (p *WhiteboardPolicy) Observe(result types.ToolResultRecord, startedAtRevision uint64) {
	if p == nil || !p.enabled {
		return
	}
	var output map[string]any
	if json.Unmarshal(result.Output, &output) != nil || output == nil || output["error"] != nil {
		return
	}
	if result.Name == "set_whiteboard" {
		// 取得完了より前に始まった表示更新では、その後に取得した情報を表示済みにしない。
		if startedAtRevision > p.written[result.GenerationID] {
			p.written[result.GenerationID] = startedAtRevision
		}
	} else if p.modes[result.Name] == tools.ToolModeRead {
		p.revision[result.GenerationID]++
	}
}

// Request は一回の情報取得結果に対する強制呼び出しを一度に制限する。
// 表示失敗時の無限リトライはせず、結果を通常の会話で説明できるようにする。
func (p *WhiteboardPolicy) Request(generation types.GenerationID) {
	if p != nil {
		p.requested[generation] = p.revision[generation]
	}
}
