package realtime

import (
	"context"
	"encoding/json"
	"github.com/tetetratra/smart-speaker/internal/types"
	"strings"
	"testing"
	"time"
)

func testInput(id, text string) types.ConversationInput {
	return types.ConversationInput{Record: types.ConversationRecord{ID: id, Role: types.RoleSystem, Source: "automation", Text: text}, External: true}
}

func TestContextAdditionDoesNotStartOrInterruptResponse(t *testing.T) {
	s := newToolResponseStage()
	s.active = "existing-response"
	before := s.cfg.Generation.Current()
	s.addContext(testInput("one", "実行済み"))
	s.addContext(testInput("one", "再送"))
	item := popToolCommand(t, s, "conversation.item.create")["item"].(map[string]any)
	if !strings.Contains(item["content"].([]any)[0].(map[string]any)["text"].(string), "実行済み") {
		t.Fatal(item)
	}
	noToolCommand(t, s)
	if s.active != "existing-response" || s.wantResponse || s.cfg.Generation.Current() != before {
		t.Fatal("情報追加だけで会話状態が変わりました")
	}
	s.transport = nil
	s.addContext(testInput("two", "未接続"))
	if s.connecting {
		t.Fatal("情報追加のために接続しました")
	}
}

func TestExternalRecordsAreUsedOnlyByAcknowledgedResponse(t *testing.T) {
	s := newToolResponseStage()
	s.addContext(testInput("before", "ライトをONにした"))
	popToolCommand(t, s, "conversation.item.create")
	s.wantResponse = true
	s.maybeRespond(context.Background())
	request := popToolCommand(t, s, "response.create")["response"].(map[string]any)
	s.addContext(testInput("after", "新しい結果"))
	popToolCommand(t, s, "conversation.item.create")
	toolServerEvent(s, map[string]any{"type": "response.created", "response": map[string]any{"id": "response", "metadata": request["metadata"]}})
	if s.externalPending["before"] || !s.externalPending["after"] {
		t.Fatal(s.externalPending)
	}
}

func TestContextAdditionPrecedesResponseRequest(t *testing.T) {
	s := newToolResponseStage()
	s.addContext(testInput("result", "処理結果"))
	s.queueInbound(testInput("remind", "結果を伝えて"))
	s.startInbound(context.Background())
	for _, text := range []string{"処理結果", "結果を伝えて"} {
		item := popToolCommand(t, s, "conversation.item.create")["item"].(map[string]any)
		if !strings.Contains(item["content"].([]any)[0].(map[string]any)["text"].(string), text) {
			t.Fatal(item)
		}
	}
	popToolCommand(t, s, "response.create")
}

func TestDisconnectedAdditionIsRestoredAndRequestRunsWithoutPeer(t *testing.T) {
	inbound := make(chan types.InboundMessage, 2)
	r := createRig(t, "__offline__", inbound)
	inbound <- types.InboundMessage{ID: "stored", Text: "外出した", Context: json.RawMessage(`{"delivery":"history_only"}`)}
	select {
	case <-r.connections:
		t.Fatal("情報追加だけで接続しました")
	case <-time.After(50 * time.Millisecond):
	}
	inbound <- types.InboundMessage{ID: "request", Text: "今の場所を教えて", Source: "automation"}
	select {
	case r.conn = <-r.connections:
	case <-r.ctx.Done():
		t.Fatal("端末なしの応答依頼で接続しませんでした")
	}
	update := r.wait("session.update")
	instructions := update["session"].(map[string]any)["instructions"].(string)
	if !strings.Contains(instructions, "外出した") || strings.Contains(instructions, "今の場所を教えて") {
		t.Fatal(instructions)
	}
	r.send(map[string]any{"type": "session.updated"})
	item := r.wait("conversation.item.create")["item"].(map[string]any)
	if text := item["content"].([]any)[0].(map[string]any)["text"].(string); text != "今の場所を教えて" {
		t.Fatal(text)
	}
	r.wait("response.create")
}

func TestConnectedAdditionIsImmediateAndDoesNotRequestResponse(t *testing.T) {
	inbound := make(chan types.InboundMessage, 1)
	r := newRigWithInbound(t, inbound)
	inbound <- types.InboundMessage{ID: "context", Text: "照明を消した", Context: json.RawMessage(`{"delivery":"history_only"}`)}
	item := r.wait("conversation.item.create")["item"].(map[string]any)
	if !strings.Contains(item["content"].([]any)[0].(map[string]any)["text"].(string), "照明を消した") {
		t.Fatal(item)
	}
	select {
	case command := <-r.commands:
		t.Fatal("情報追加が余分なAPI要求を発行しました", command)
	case <-time.After(30 * time.Millisecond):
	}
}
