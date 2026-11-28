package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/tetetratra/smart-speaker/internal/types"
	"strings"
	"testing"
	"time"
)

func testInput(id, text string) types.ConversationInput {
	return types.ConversationInput{Record: types.ConversationRecord{ID: id, Role: types.RoleSystem, Source: "automation", Text: text, External: true}}
}

func TestContextAdditionDoesNotStartOrInterruptResponse(t *testing.T) {
	s := newTestStage(t)
	s.active = "existing-response"
	before := s.cfg.Generation.Current()
	s.addContext(testInput("one", "実行済み"))
	s.addContext(testInput("one", "再送"))
	item := popCommand(t, s, "conversation.item.create")["item"].(map[string]any)
	if !strings.Contains(item["content"].([]any)[0].(map[string]any)["text"].(string), "実行済み") {
		t.Fatal(item)
	}
	noCommand(t, s)
	if s.active != "existing-response" || len(s.pendingRequests) > 0 || s.cfg.Generation.Current() != before {
		t.Fatal("情報追加だけで会話状態が変わりました")
	}
	s.transport = nil
	s.addContext(testInput("two", "未接続"))
	if s.connecting {
		t.Fatal("情報追加のために接続しました")
	}
}

func TestExternalRecordsAreUsedOnlyByAcknowledgedResponse(t *testing.T) {
	s := newTestStage(t)
	s.addContext(testInput("before", "ライトをONにした"))
	popCommand(t, s, "conversation.item.create")
	s.pendingRequests = []types.ResponseRequest{{GenerationID: s.cfg.Generation.Current()}}
	s.maybeRespond(context.Background())
	request := popCommand(t, s, "response.create")["response"].(map[string]any)
	s.addContext(testInput("after", "新しい結果"))
	popCommand(t, s, "conversation.item.create")
	serverEventForTest(s, map[string]any{"type": "response.created", "response": map[string]any{"id": "response", "metadata": request["metadata"]}})
	if s.externalPending["before"] || !s.externalPending["after"] {
		t.Fatal(s.externalPending)
	}
}

// ターンの外の応答依頼（頭脳AIが後から届いた結果に対応した内容）は、先に入れた参考情報の後に、新しい世代で送る。
func TestContextAdditionPrecedesOutOfTurnResponse(t *testing.T) {
	s := newTestStage(t)
	before := s.cfg.Generation.Current()
	s.addContext(testInput("result", "処理結果"))
	s.handle(context.Background(), types.EventResponseRequest{ResponseRequest: types.ResponseRequest{Notice: "結果を伝えて", NewGeneration: true}})
	item := popCommand(t, s, "conversation.item.create")["item"].(map[string]any)
	if !strings.Contains(item["content"].([]any)[0].(map[string]any)["text"].(string), "処理結果") {
		t.Fatal(item)
	}
	response, notice := popResponse(t, s)
	if !strings.Contains(notice, "結果を伝えて") || s.cfg.Generation.Current() == before {
		t.Fatal("新しい世代で依頼の知らせを付けて応答していません", notice)
	}
	if response["metadata"].(map[string]string)["generation"] != fmt.Sprint(s.cfg.Generation.Current()) {
		t.Fatal(response)
	}
}

// 端末がつながっていなくても、応答依頼が届いたら接続し、履歴を復元してから応答する。
func TestDisconnectedAdditionIsRestoredAndRequestRunsWithoutPeer(t *testing.T) {
	inbound := make(chan types.ConversationInput, 1)
	r := createRig(t, "__offline__", inbound)
	inbound <- types.ExternalInput(types.ConversationRecord{ID: "stored", Text: "外出した"}, json.RawMessage(`{"delivery":"history_only"}`))
	select {
	case <-r.connections:
		t.Fatal("情報追加だけで接続しました")
	case <-time.After(50 * time.Millisecond):
	}
	r.node.Upstream <- types.EventResponseRequest{ResponseRequest: types.ResponseRequest{Notice: "今の場所を伝えて", NewGeneration: true}}
	select {
	case r.conn = <-r.connections:
	case <-r.ctx.Done():
		t.Fatal("端末なしの応答依頼で接続しませんでした")
	}
	r.wait("session.update")
	r.send(map[string]any{"type": "session.updated"})
	restored := r.wait("conversation.item.create")["item"].(map[string]any)
	if text := restored["content"].([]any)[0].(map[string]any)["text"].(string); !strings.Contains(text, "外出した") {
		t.Fatal(text)
	}
	if _, notice := r.waitResponse(); !strings.Contains(notice, "今の場所を伝えて") {
		t.Fatal(notice)
	}
}

func TestConnectedAdditionIsImmediateAndDoesNotRequestResponse(t *testing.T) {
	inbound := make(chan types.ConversationInput, 1)
	r := newRigWithInbound(t, inbound)
	inbound <- types.ExternalInput(types.ConversationRecord{ID: "context", Text: "照明を消した"}, json.RawMessage(`{"delivery":"history_only"}`))
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
