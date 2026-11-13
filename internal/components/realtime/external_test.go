package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func externalMessage(id, text string) types.InboundMessage {
	return types.InboundMessage{ID: id, Source: "automation", Text: text, Context: json.RawMessage(`{"delivery":"history_only","call_id":"call"}`)}
}

func TestExternalRecordDoesNotStartOrInterruptResponse(t *testing.T) {
	s := newToolResponseStage()
	s.cfg.ToolHistory = toolhistory.NewStore(10)
	s.active = "existing-response"
	before := s.cfg.Generation.Current()
	s.recordExternal(context.Background(), externalMessage("one", "実行済み"))
	s.recordExternal(context.Background(), externalMessage("one", "再送"))
	if s.active != "existing-response" || s.wantResponse || s.cfg.Generation.Current() != before {
		t.Fatal("記録だけで会話状態が変わりました")
	}
	noToolCommand(t, s)
	if len(s.cfg.History.Snapshot()) != 1 || len(s.cfg.ToolHistory.Snapshot()) != 1 {
		t.Fatal("再送が重複記録されました")
	}
	s.transport = nil
	s.recordExternal(context.Background(), externalMessage("two", "未接続"))
	if s.connecting {
		t.Fatal("記録のために接続しました")
	}
}

func TestExternalRecordsAreUsedOnlyByAcknowledgedResponse(t *testing.T) {
	s := newToolResponseStage()
	s.recordExternal(context.Background(), externalMessage("before", "ライトをONにした"))
	// 接続だけで会話が終わっても、未利用記録は残す。
	s.cfg.History.DrainExcept(s.externalPending)
	if len(s.cfg.History.Snapshot()) != 1 {
		t.Fatal("未利用の履歴を破棄しました")
	}
	s.wantResponse = true
	s.maybeRespond(context.Background())
	item := popToolCommand(t, s, "conversation.item.create")["item"].(map[string]any)
	if !strings.Contains(item["content"].([]any)[0].(map[string]any)["text"].(string), "ライトをONにした") {
		t.Fatal(item)
	}
	request := popToolCommand(t, s, "response.create")["response"].(map[string]any)
	s.recordExternal(context.Background(), externalMessage("after", "新しい結果"))
	if !s.externalPending["before"] {
		t.Fatal("応答開始確認前に利用済みになりました")
	}
	toolServerEvent(s, map[string]any{"type": "response.created", "response": map[string]any{"id": "response", "metadata": request["metadata"]}})
	if s.externalPending["before"] || !s.externalPending["after"] {
		t.Fatal("要求時点の記録だけを利用済みにできていません", s.externalPending)
	}
	drained := s.cfg.History.DrainExcept(s.externalPending)
	if len(drained) != 1 || drained[0].ID != "before" {
		t.Fatal(drained)
	}
	if got := s.cfg.History.Snapshot(); len(got) != 1 || got[0].ID != "after" {
		t.Fatal(got)
	}
}

func TestExternalRecordsPrecedeRemindAndSurviveIdle(t *testing.T) {
	s := newToolResponseStage()
	s.cfg.IdleTimeout = time.Second
	s.lastActivity = time.Now().Add(-2 * time.Second)
	s.recordExternal(context.Background(), externalMessage("result", "処理結果"))
	// transportなしならidleによるrotateも接続もしない。
	s.transport = nil
	s.tick(context.Background(), time.Now())
	if len(s.cfg.History.Snapshot()) != 1 || s.connecting {
		t.Fatal("idleで記録が失われたか接続しました")
	}
	// 同一Ingressの順に受信し、先行する記録をremindの応答前に渡す。
	s = newToolResponseStage()
	s.recordExternal(context.Background(), externalMessage("result", "処理結果"))
	s.queueInbound(types.InboundMessage{Source: "automation", Text: "結果を伝えて"})
	s.startInbound(context.Background())
	popToolCommand(t, s, "conversation.item.create") // remindの指示
	item := popToolCommand(t, s, "conversation.item.create")["item"].(map[string]any)
	if !strings.Contains(item["content"].([]any)[0].(map[string]any)["text"].(string), "処理結果") {
		t.Fatal(item)
	}
	popToolCommand(t, s, "response.create")
}

func TestAutomationRecordsAvoidDuplicateToolEntries(t *testing.T) {
	s := newToolResponseStage()
	s.cfg.ToolHistory = toolhistory.NewStore(10)
	for i, role := range []string{types.RoleToolCall, types.RoleToolResult, types.RoleSystem, ""} {
		message := externalMessage(fmt.Sprint(i), "予約の記録")
		message.Role = role
		message.Title = "室温"
		s.recordExternal(context.Background(), message)
	}
	entries := s.cfg.ToolHistory.Snapshot()
	if len(entries) != 2 {
		t.Fatal("呼び出しと受付結果が重複表示されました", entries)
	}
	for _, entry := range entries {
		if entry.Name != "automation" || entry.Title != "室温" {
			t.Fatal(entry)
		}
	}
}
