package conversationmanager

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func testStage() *stage {
	return &stage{node: &graph.Stage{Downstream: make(chan types.Event, 20)}, history: conversationhistory.NewStore(), seen: map[string]bool{}, unused: map[string]bool{}}
}

func TestRecordsOnlyUpdateHistory(t *testing.T) {
	s := testStage()
	for _, text := range []string{"仮の文字起こし", "確定した文字起こし"} {
		s.handle(context.Background(), types.Event{Kind: types.EventConversationRecord, Payload: types.ConversationRecord{ID: "speech", Text: text}})
	}
	records := s.history.Snapshot()
	if len(records) != 1 || records[0].Text != "確定した文字起こし" {
		t.Fatal(records)
	}
	select {
	case e := <-s.node.Downstream:
		t.Fatal("記録をAPIへ送り返しました", e)
	default:
	}
}

func TestInputIsSavedBeforeForwardingAndDeliveryIsNotStored(t *testing.T) {
	for _, quiet := range []bool{false, true} {
		s := testStage()
		s.cfg.ToolHistory = toolhistory.NewStore(10)
		raw := json.RawMessage(`{"automation_id":"job"}`)
		want := types.EventConversationResponseRequest
		if quiet {
			raw = json.RawMessage(`{"delivery":"history_only","automation_id":"job"}`)
			want = types.EventConversationContextAddition
		}
		e := InputEvent(types.InboundMessage{ID: "result", Source: "tool/web_search", Title: "天気", Text: "検索結果", Context: raw})
		s.handle(context.Background(), e)
		s.handle(context.Background(), e)
		forwarded := <-s.node.Downstream
		records := s.history.Snapshot()
		if forwarded.Kind != want || len(records) != 1 || records[0].Text != "検索結果" {
			t.Fatal(forwarded, records)
		}
		metadata := records[0].Metadata["context"].(string)
		if strings.Contains(metadata, "delivery") || !strings.Contains(metadata, "job") {
			t.Fatal(metadata)
		}
		if entries := s.cfg.ToolHistory.Snapshot(); len(entries) != 1 || entries[0].Title != "天気" {
			t.Fatal(entries)
		}
		select {
		case e := <-s.node.Downstream:
			t.Fatal("再送を二度配信しました", e)
		default:
		}
	}
}

func TestSnapshotReturnsAllHistoryAndMemoryWithoutReplaying(t *testing.T) {
	s := testStage()
	s.cfg.MemoryContext = func(context.Context) (string, error) { return "長期メモリ", nil }
	s.handle(context.Background(), InputEvent(types.InboundMessage{ID: "input", Text: "依頼"}))
	<-s.node.Downstream
	s.handle(context.Background(), types.Event{Kind: types.EventConversationSnapshotRequest, Payload: types.ConversationSnapshotRequest{Epoch: 7}})
	e := <-s.node.Downstream
	snapshot := e.Payload.(types.ConversationSnapshot)
	if e.Kind != types.EventConversationSnapshot || snapshot.Epoch != 7 || len(snapshot.Records) != 1 || snapshot.Memory != "長期メモリ" {
		t.Fatal(e)
	}
	select {
	case e := <-s.node.Downstream:
		t.Fatal("応答依頼を再発火しました", e)
	default:
	}
}

func TestConversationEndKeepsUnusedRecordsAndPassesFixedSnapshot(t *testing.T) {
	s := testStage()
	snapshots := make(chan []types.ConversationRecord, 1)
	s.cfg.OnConversationEnd = func(_ context.Context, records []types.ConversationRecord) error { snapshots <- records; return nil }
	for _, id := range []string{"used", "unused"} {
		s.handle(context.Background(), InputEvent(types.InboundMessage{ID: id, Text: id}))
		<-s.node.Downstream
	}
	s.handle(context.Background(), types.Event{Kind: types.EventConversationRecordsUsed, Payload: types.ConversationRecordsUsed{IDs: []string{"used"}}})
	s.handle(context.Background(), types.Event{Kind: types.EventConversationEnded})
	s.handle(context.Background(), types.Event{Kind: types.EventConversationRecord, Payload: types.ConversationRecord{ID: "new", Text: "新しい会話"}})
	records := <-snapshots
	if len(records) != 1 || records[0].ID != "used" {
		t.Fatal(records)
	}
	remaining := s.history.Snapshot()
	if len(remaining) != 2 || remaining[0].ID != "unused" || remaining[1].ID != "new" {
		t.Fatal(remaining)
	}
}

func TestAutomationToolCallsAreNotDisplayedTwice(t *testing.T) {
	s := testStage()
	s.cfg.ToolHistory = toolhistory.NewStore(10)
	for _, role := range []string{types.RoleToolCall, types.RoleToolResult, types.RoleSystem} {
		s.handle(context.Background(), InputEvent(types.InboundMessage{ID: role, Role: role, Source: "automation", Title: "室温", Context: json.RawMessage(`{"delivery":"history_only"}`)}))
		<-s.node.Downstream
	}
	if entries := s.cfg.ToolHistory.Snapshot(); len(entries) != 1 || entries[0].Title != "室温" {
		t.Fatal(entries)
	}
}
