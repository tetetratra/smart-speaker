package toolhistory

import (
	"encoding/json"
	"github.com/tetetratra/smart-speaker/internal/types"
	"testing"
)

func TestRetentionAndSnapshots(t *testing.T) {
	s := NewStore(2)
	first := s.Begin(types.ToolRequest{ToolCallID: "1", Name: "first", Arguments: json.RawMessage(`{"query":"日本語"}`)})
	s.Finish(first, types.ToolResultRecord{Output: json.RawMessage(`{"result":"全文"}`)})
	second := s.Begin(types.ToolRequest{ToolCallID: "2", Name: "second"})
	s.Finish(second, types.ToolResultRecord{Output: json.RawMessage(`{"error":"失敗"}`)})
	third := s.Begin(types.ToolRequest{ToolCallID: "3", Name: "third"})
	entries := s.Snapshot()
	if len(entries) != 2 || entries[0].ID != third || entries[0].Status != "running" || entries[1].Status != "failed" {
		t.Fatal(entries)
	}
	entries[0].Name = "外から変更"
	if s.Snapshot()[0].Name != "third" {
		t.Fatal("スナップショットから内部状態が変更されました")
	}
	if len(NewStore(2).Snapshot()) != 0 {
		t.Fatal("別の実行へ履歴を引き継ぎました")
	}
}

func TestSubscriptionKeepsLatestStateWithoutBlockingWriters(t *testing.T) {
	s := NewStore(2)
	changed, unsubscribe := s.Subscribe()
	other, stopOther := s.Subscribe()
	defer stopOther()
	<-changed
	<-other
	id := s.Begin(types.ToolRequest{Name: "検索"})
	<-changed
	if s.Snapshot()[0].Status != "running" {
		t.Fatal("開始状態が記録されていません")
	}
	// 遅い購読者がいても、終了の記録を止めない。
	s.Finish(id, types.ToolResultRecord{Output: json.RawMessage(`{"result":"全文"}`)})
	<-changed
	<-other
	if s.Snapshot()[0].Status != "succeeded" {
		t.Fatal("最新の終了状態を取得できません")
	}
	unsubscribe()
	unsubscribe()
	s.Begin(types.ToolRequest{Name: "次の検索"})
	select {
	case <-changed:
		t.Fatal("購読解除後に通知されました")
	default:
	}
	<-other
}
