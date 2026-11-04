package conversation

import (
	"encoding/json"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/types"
	"testing"
)

func TestReadResultsRequireWhiteboardBeforeFollowUpButWritesAndErrorsDoNot(t *testing.T) {
	p := NewWhiteboardPolicy(map[string]string{"web_search": tools.ToolModeRead, "calendar_create": tools.ToolModeWrite}, true)
	result := types.ToolResultRecord{Name: "web_search", GenerationID: 1, Output: json.RawMessage(`{"result":"取得した情報"}`)}
	p.Observe(result, 0)
	if !p.Pending(1) || p.Pending(2) {
		t.Fatal("情報取得後の表示要求が世代ごとに管理されない")
	}
	p.Observe(types.ToolResultRecord{Name: "set_whiteboard", GenerationID: 1, Output: json.RawMessage(`{"updated":true}`)}, p.Revision(1))
	if p.Pending(1) {
		t.Fatal("正常更新後も強制呼び出しを繰り返す")
	}
	result.Output = json.RawMessage(`{"error":"失敗"}`)
	p.Observe(result, 0)
	result.Name = "calendar_create"
	result.Output = json.RawMessage(`{"ok":true}`)
	p.Observe(result, 0)
	if p.Pending(1) {
		t.Fatal("失敗や操作成功でも情報取得扱いになった")
	}
}
func TestEarlyWhiteboardCallDoesNotAcknowledgeLaterReadResult(t *testing.T) {
	p := NewWhiteboardPolicy(map[string]string{"web_search": tools.ToolModeRead}, true)
	before := p.Revision(1)
	p.Observe(types.ToolResultRecord{Name: "web_search", GenerationID: 1, Output: json.RawMessage(`{"result":"最新情報"}`)}, 0)
	p.Observe(types.ToolResultRecord{Name: "set_whiteboard", GenerationID: 1, Output: json.RawMessage(`{"updated":true}`)}, before)
	if !p.Pending(1) {
		t.Fatal("取得前に作った表示で取得後の情報まで表示済みになった")
	}
}

func TestFailedForcedWriteDoesNotCauseInfiniteResponseLoop(t *testing.T) {
	p := NewWhiteboardPolicy(map[string]string{"web_search": tools.ToolModeRead}, true)
	result := types.ToolResultRecord{Name: "web_search", GenerationID: 1, Output: json.RawMessage(`{"result":"情報"}`)}
	p.Observe(result, 0)
	if !p.Pending(1) {
		t.Fatal("表示要求なし")
	}
	p.Request(1)
	p.Observe(types.ToolResultRecord{Name: "set_whiteboard", GenerationID: 1, Output: json.RawMessage(`{"error":"表示失敗"}`)}, p.Revision(1))
	if p.Pending(1) {
		t.Fatal("表示失敗を強制的にリトライし続ける")
	}
	p.Observe(result, 0)
	if !p.Pending(1) {
		t.Fatal("次の情報取得まで抑制された")
	}
}
