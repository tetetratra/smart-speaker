package conversationmanager

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func testStage(t *testing.T) *stage {
	t.Helper()
	history, err := conversationhistory.NewStore(filepath.Join(t.TempDir(), "conversation_history.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &stage{node: &graph.Stage{Downstream: make(chan types.Event, 20)}, history: history, seen: map[string]bool{}, unused: map[string]bool{}}
}

func TestRecordsOnlyUpdateHistory(t *testing.T) {
	s := testStage(t)
	for _, text := range []string{"仮の文字起こし", "確定した文字起こし"} {
		s.handle(context.Background(), types.Event{Kind: types.EventConversationInput, Payload: types.ConversationInput{Record: types.ConversationRecord{ID: "speech", Role: types.RoleUser, Text: text}, ModelHandling: types.ModelHandlingAlreadyKnown}})
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
		s := testStage(t)
		raw := json.RawMessage(`{"automation_id":"job"}`)
		want := types.ModelHandlingAddAndRespond
		if quiet {
			raw = json.RawMessage(`{"delivery":"history_only","automation_id":"job"}`)
			want = types.ModelHandlingAddToContext
		}
		e := InputEvent(types.InboundMessage{ID: "result", Source: "tool/web_search", Title: "天気", Text: "検索結果", Context: raw})
		s.handle(context.Background(), e)
		s.handle(context.Background(), e)
		forwarded := nextForwarded(s)
		records := s.history.Snapshot()
		if forwarded.Kind != types.EventConversationInput || forwarded.Payload.(types.ConversationInput).ModelHandling != want || len(records) != 1 || records[0].Text != "検索結果" {
			t.Fatal(forwarded, records)
		}
		metadata := records[0].Metadata["context"].(string)
		if strings.Contains(metadata, "delivery") || !strings.Contains(metadata, "job") {
			t.Fatal(metadata)
		}
		if records[0].Title != "天気" || !records[0].External {
			t.Fatal("表示用のタイトルと外部からの入力の印を記録に持つ必要があります", records[0])
		}
		for len(s.node.Downstream) > 0 {
			if e := <-s.node.Downstream; e.Kind != types.EventConversationRecordChanged {
				t.Fatal("再送を二度配信しました", e)
			}
		}
	}
}

func TestHistorySyncPreservesDeliveryAndReadOrder(t *testing.T) {
	s := testStage(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s.handle(ctx, InputEvent(types.InboundMessage{ID: "request", Text: "通常入力として渡す依頼"}))
	done, returned := make(chan struct{}), make(chan struct{})
	go func() {
		s.handle(ctx, types.Event{Kind: types.EventConversationSignal, Payload: types.ConversationHistorySync{Epoch: 1, Done: done}})
		close(returned)
	}()
	// 保存済みの応答依頼が、読み出しマーカーより先に配送される。
	if e := nextForwarded(s); e.Kind != types.EventConversationInput {
		t.Fatal(e)
	}
	if e := <-s.node.Downstream; e.Kind != types.EventConversationSignal || !isHistorySync(e) {
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
	s := testStage(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.handle(ctx, types.Event{Kind: types.EventConversationSignal, Payload: types.ConversationHistorySync{Done: make(chan struct{})}})
}

func TestIdlePassesOnlyUnusedRecordIDsAndKeepsHistory(t *testing.T) {
	s := testStage(t)
	var keep map[string]bool
	s.cfg.OnIdle = func(_ context.Context, unused map[string]bool) { keep = unused }
	for _, id := range []string{"used", "unused"} {
		s.handle(context.Background(), InputEvent(types.InboundMessage{ID: id, Text: id}))
		<-s.node.Downstream
	}
	s.handle(context.Background(), types.Event{Kind: types.EventConversationSignal, Payload: types.ConversationRecordsUsed{IDs: []string{"used"}}})
	s.handle(context.Background(), types.Event{Kind: types.EventConversationSignal, Payload: types.ConversationIdle{}})
	if len(keep) != 1 || !keep["unused"] {
		t.Fatal(keep)
	}
	// 無操作の通知だけでは履歴を消さない。
	if records := s.history.Snapshot(); len(records) != 2 {
		t.Fatal(records)
	}
}

func isHistorySync(e types.Event) bool {
	_, ok := e.Payload.(types.ConversationHistorySync)
	return ok
}

// nextForwarded は、画面への記録の通知を飛ばして、realtime へ渡すイベントを取り出す。
func nextForwarded(s *stage) types.Event {
	for {
		if e := <-s.node.Downstream; e.Kind != types.EventConversationRecordChanged {
			return e
		}
	}
}

func TestOnlyNonDialogueRecordsAreSentToScreen(t *testing.T) {
	s := testStage(t)
	for _, record := range []types.ConversationRecord{
		{ID: "user", Role: types.RoleUser, Text: "こんにちは"},
		{ID: "agent", Role: types.RoleAgent, Text: "はい"},
		{ID: "call", Role: types.RoleToolCall, ToolName: "set_whiteboard", CallID: "c1"},
		{ID: "result", Role: types.RoleToolResult, ToolName: "set_whiteboard", CallID: "c1"},
	} {
		s.handle(context.Background(), types.Event{Kind: types.EventConversationInput, Payload: types.ConversationInput{Record: record, ModelHandling: types.ModelHandlingAlreadyKnown}})
	}
	var sent []string
	for len(s.node.Downstream) > 0 {
		e := <-s.node.Downstream
		if e.Kind != types.EventConversationRecordChanged {
			t.Fatal(e)
		}
		sent = append(sent, e.Payload.(types.ConversationRecord).ID)
	}
	if strings.Join(sent, ",") != "call,result" {
		t.Fatal(sent)
	}
	if len(s.history.Snapshot()) != 4 {
		t.Fatal("会話の記録も保存する必要があります", s.history.Snapshot())
	}
}
