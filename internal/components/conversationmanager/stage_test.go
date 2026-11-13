package conversationmanager

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

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

func TestHistorySyncPreservesDeliveryAndReadOrder(t *testing.T) {
	s := testStage()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s.handle(ctx, InputEvent(types.InboundMessage{ID: "request", Text: "通常入力として渡す依頼"}))
	done, returned := make(chan struct{}), make(chan struct{})
	go func() {
		s.handle(ctx, types.Event{Kind: types.EventConversationHistorySync, Payload: types.ConversationHistorySync{Epoch: 1, Done: done}})
		close(returned)
	}()
	// 保存済みの応答依頼が、読み出しマーカーより先に配送される。
	if e := <-s.node.Downstream; e.Kind != types.EventConversationResponseRequest {
		t.Fatal(e)
	}
	if e := <-s.node.Downstream; e.Kind != types.EventConversationHistorySync {
		t.Fatal(e)
	}
	select {
	case <-returned:
		t.Fatal("読み出し前に後続処理を再開した")
	default:
	}
	if records := s.history.Snapshot(); len(records) != 1 || records[0].ID != "request" {
		t.Fatal(records)
	}
	close(done)
	select {
	case <-returned:
	case <-ctx.Done():
		t.Fatal("読み出し後に再開しない")
	}
}

func TestHistorySyncCanBeCancelled(t *testing.T) {
	s := testStage()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.handle(ctx, types.Event{Kind: types.EventConversationHistorySync, Payload: types.ConversationHistorySync{Done: make(chan struct{})}})
}

func TestConversationEndKeepsUnusedRecordsAndPassesFixedSnapshot(t *testing.T) {
	s := testStage()
	snapshots := make(chan []types.ConversationRecord, 1)
	s.cfg.OnConversationEnd = func(_ context.Context, keep map[string]bool) { snapshots <- s.history.DrainExcept(keep) }
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
