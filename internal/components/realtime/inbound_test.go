package realtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func TestInboundMessageWaitsForConversation(t *testing.T) {
	s := &stage{speaking: true}
	s.queueInbound(types.InboundMessage{Source: "automation", Text: "室温を知らせて"})
	if s.startInbound(context.Background()) || len(s.inbound) != 1 {
		t.Fatal(s.inbound)
	}
}

// 時間のかかるツールの実行中でも、予約ジョブは時刻どおりに始める。再生端末がつながっていなくても待たない。
func TestInboundMessageStartsWhileToolIsRunning(t *testing.T) {
	s := newToolResponseStage()
	s.calls["slow"] = &call{epoch: 1, generation: 1}
	s.queueInbound(types.InboundMessage{Source: "automation", Text: "室温を知らせて"})
	if !s.startInbound(context.Background()) || len(s.inbound) != 0 {
		t.Fatal(s.inbound)
	}
}

// 接続準備が済むまでは会話履歴・表示履歴へ出さず、準備後に同じ本文を一度だけ記録する。
func TestInboundHistoryWaitsForConnection(t *testing.T) {
	s := newToolResponseStage()
	s.ready = false
	s.cfg.ToolHistory = toolhistory.NewStore(10)
	message := types.InboundMessage{Source: "future/source", Title: "結果のタイトル", Context: json.RawMessage(`{"automation_id":"job-1","run_id":"run-1"}`), Text: "全文\n<script>外部データ</script>"}
	s.queueInbound(message)
	if !s.startInbound(context.Background()) || len(s.pendingText) != 1 {
		t.Fatal("接続準備待ちの入力になっていません")
	}
	if len(s.cfg.History.Snapshot()) != 0 || len(s.cfg.ToolHistory.Snapshot()) != 0 {
		t.Fatal("送信準備が済む前に履歴へ記録しました")
	}
	toolServerEvent(s, map[string]any{"type": "session.updated"})
	item := popToolCommand(t, s, "conversation.item.create")["item"].(map[string]any)
	if got := item["content"].([]any)[0].(map[string]any)["text"]; got != message.Text {
		t.Fatal("モデルへ渡す本文が変わりました", got)
	}
	entries, records := s.cfg.ToolHistory.Snapshot(), s.cfg.History.Snapshot()
	if len(entries) != 1 || entries[0].Kind != "inbound" || entries[0].Source != message.Source || entries[0].Name != message.Source || entries[0].Title != message.Title || entries[0].Text != message.Text {
		t.Fatal(entries)
	}
	if len(records) != 1 || records[0].Role != types.RoleSystem || records[0].Metadata["context"] != string(message.Context) || records[0].Source != message.Source || records[0].Text != message.Text || records[0].GenerationID != entries[0].GenerationID {
		t.Fatal(records)
	}
	toolServerEvent(s, map[string]any{"type": "session.updated"})
	if len(s.cfg.ToolHistory.Snapshot()) != 1 {
		t.Fatal("同じ差し込みを二度記録しました")
	}
}

func TestInboundWaitsForEachBusyState(t *testing.T) {
	cases := map[string]func(*stage){
		"発話中":  func(s *stage) { s.speaking = true },
		"応答中":  func(s *stage) { s.active = "response" },
		"応答待ち": func(s *stage) { s.wantResponse = true },
		"接続中":  func(s *stage) { s.connecting = true },
		"入力保留": func(s *stage) { s.pendingText = []types.LLMRequest{{Text: "入力"}} },
		"修復中":  func(s *stage) { s.repairs = map[string]bool{"item": true} },
		"出力待ち": func(s *stage) {
			s.pendingOutput = map[string]bool{"item": true}
			s.items = map[string]*assistantItem{"item": {generation: s.cfg.Generation.Current()}}
		},
	}
	for name, busy := range cases {
		t.Run(name, func(t *testing.T) {
			s := newToolResponseStage()
			busy(s)
			s.queueInbound(types.InboundMessage{Source: "tool/web_search", Text: "結果"})
			if s.startInbound(context.Background()) || len(s.inbound) != 1 {
				t.Fatal("処理中に差し込みました")
			}
		})
	}
}

func TestInboundKeepsFIFOAndUserInputIsNotDisplayed(t *testing.T) {
	s := newToolResponseStage()
	s.cfg.ToolHistory = toolhistory.NewStore(10)
	for _, text := range []string{"最初", "次"} {
		s.queueInbound(types.InboundMessage{Source: "automation", Text: text})
	}
	if !s.startInbound(context.Background()) || s.startInbound(context.Background()) || len(s.inbound) != 1 {
		t.Fatal("応答を待たずに複数差し込みました")
	}
	s.active, s.wantResponse = "", false
	if !s.startInbound(context.Background()) || len(s.inbound) != 0 {
		t.Fatal("次の差し込みが残っています")
	}
	entries := s.cfg.ToolHistory.Snapshot()
	if len(entries) != 2 || entries[0].Text != "次" || entries[1].Text != "最初" || entries[0].ID == entries[1].ID {
		t.Fatal("同じ出どころの差し込みが失われたか、順番が変わりました", entries)
	}
	s.handle(context.Background(), types.Event{Payload: types.LLMRequest{Text: "ユーザーの入力"}})
	if len(s.cfg.ToolHistory.Snapshot()) != 2 {
		t.Fatal("ユーザーの入力を差し込みとして表示しました")
	}
}
