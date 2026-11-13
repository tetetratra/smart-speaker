package realtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/components/generationcontroller"
	"github.com/tetetratra/smart-speaker/internal/eventqueue"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func newToolResponseStage() *stage {
	gen := generation.NewStore()
	gen.Next()
	return &stage{
		cfg:        Config{Generation: gen},
		node:       &graph.Stage{Downstream: make(chan types.Event, 100)},
		transport:  &transport{commands: eventqueue.New[any](nil)},
		controller: generationcontroller.New(gen), epoch: 1, ready: true,
		instructions: "元の会話指示", started: time.Now(),
		calls:     map[string]*call{"search": {epoch: 1, generation: 1}},
		responses: map[string]*response{}, userGenerations: map[string]types.GenerationID{},
	}
}

func toolResultForTest(id, name, output string) types.ToolResultRecord {
	return types.ToolResultRecord{ToolCallID: id, Name: name, GenerationID: 1, Output: json.RawMessage(output)}
}

func popToolCommand(t *testing.T, s *stage, kind string) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd, ok := s.transport.commands.Pop(ctx)
	if !ok || cmd.(map[string]any)["type"] != kind {
		t.Fatalf("期待するAPI要求がありません: %s, %v", kind, cmd)
	}
	return cmd.(map[string]any)
}

func noToolCommand(t *testing.T, s *stage) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if cmd, ok := s.transport.commands.Pop(ctx); ok {
		t.Fatalf("不要なAPI要求です: %v", cmd)
	}
}

func toolServerEvent(s *stage, event any) {
	raw, _ := json.Marshal(event)
	s.serverEvent(context.Background(), raw)
}

func TestToolInstructionsWaitForCurrentInputCommit(t *testing.T) {
	s := newToolResponseStage()
	toolServerEvent(s, map[string]any{"type": "input_audio_buffer.speech_started", "item_id": "user2"})
	s.toolResult(context.Background(), toolResultForTest("search", "web_search", `{"result":"晴れ"}`))
	popToolCommand(t, s, "conversation.item.create")
	noToolCommand(t, s)
	toolServerEvent(s, map[string]any{"type": "input_audio_buffer.speech_stopped", "item_id": "user2"})
	// 直前の発話の遅い確定通知やtickでは、現在の入力確定待ちを解除しない。
	toolServerEvent(s, map[string]any{"type": "input_audio_buffer.committed", "item_id": "user1"})
	s.tick(context.Background(), time.Now())
	noToolCommand(t, s)
	if len(s.pendingToolResponses) != 1 {
		t.Fatal("待機中に追加指示が失われました")
	}
	toolServerEvent(s, map[string]any{"type": "input_audio_buffer.committed", "item_id": "user2"})
	response := popToolCommand(t, s, "response.create")["response"].(map[string]any)
	instructions := response["instructions"].(string)
	for _, text := range []string{"元の会話指示", "search", "取り消された依頼", "口頭説明より先に"} {
		if !strings.Contains(instructions, text) {
			t.Fatalf("追加指示に必要な内容がありません: %s", text)
		}
	}
	if response["metadata"].(map[string]string)["generation"] != "2" {
		t.Fatal("旧世代で応答しています", response)
	}
	if len(s.pendingToolResponses) != 0 {
		t.Fatal("送信後も一回用の指示が残っています")
	}
	// 同じ結果の再配信は次の応答を起動しない。
	s.toolResult(context.Background(), toolResultForTest("search", "web_search", `{"result":"晴れ"}`))
	noToolCommand(t, s)
}

func TestToolInstructionsWaitForActiveResponseAndRepairs(t *testing.T) {
	s := newToolResponseStage()
	s.active = "ack"
	s.repairs = map[string]bool{"old-item": true}
	s.toolResult(context.Background(), toolResultForTest("search", "web_search", `{"result":"晴れ"}`))
	popToolCommand(t, s, "conversation.item.create")
	toolServerEvent(s, map[string]any{"type": "response.done", "response": map[string]any{"id": "ack"}})
	noToolCommand(t, s)
	toolServerEvent(s, map[string]any{"type": "conversation.item.deleted", "item_id": "old-item"})
	response := popToolCommand(t, s, "response.create")["response"].(map[string]any)
	if !strings.Contains(response["instructions"].(string), `"call_id":"search"`) {
		t.Fatal("履歴補正中にツール結果の案内が失われました")
	}
}

// 同じ応答で出た別の呼び出しが実行中でも、届いた結果から先に応答する。
func TestToolResultRespondsWhileOtherCallIsRunning(t *testing.T) {
	s := newToolResponseStage()
	s.calls["slow"] = &call{epoch: 1, generation: 1}
	s.toolResult(context.Background(), toolResultForTest("search", "web_search", `{"result":"情報"}`))
	popToolCommand(t, s, "conversation.item.create")
	response := popToolCommand(t, s, "response.create")["response"].(map[string]any)
	if !strings.Contains(response["instructions"].(string), `"call_id":"search"`) {
		t.Fatal(response)
	}
}

func TestReconnectedToolResultUsesContextAndKeepsInstructions(t *testing.T) {
	s := newToolResponseStage()
	s.epoch = 2
	s.ready = false
	s.awaitingInputCommit = ""
	s.toolResult(context.Background(), toolResultForTest("search", "web_search", `{"result":"晴れ"}`))
	noToolCommand(t, s)
	toolServerEvent(s, map[string]any{"type": "session.updated"})
	item := popToolCommand(t, s, "conversation.item.create")["item"].(map[string]any)
	if item["type"] != "message" {
		t.Fatal("旧call_idの結果を新しい接続へ再送しました", item)
	}
	response := popToolCommand(t, s, "response.create")["response"].(map[string]any)
	if !strings.Contains(response["instructions"].(string), "search") {
		t.Fatal("再接続時に追加指示がありません")
	}
}

func TestToolResultBetweenDisconnectAndReconnectIsRetained(t *testing.T) {
	s := newToolResponseStage()
	s.epoch = 2
	s.ready = false
	transport := s.transport
	s.transport = nil
	s.peers = map[string]bool{"phone": true}
	s.toolResult(context.Background(), toolResultForTest("search", "web_search", `{"result":"晴れ"}`))
	if len(s.contextResults) != 1 || len(s.pendingToolResponses) != 1 {
		t.Fatal("再接続開始前の結果が失われました")
	}
	s.transport = transport
	toolServerEvent(s, map[string]any{"type": "session.updated"})
	item := popToolCommand(t, s, "conversation.item.create")["item"].(map[string]any)
	encoded, _ := json.Marshal(item)
	if !strings.Contains(string(encoded), "call_id=search") {
		t.Fatal("再接続した結果の識別子がありません", item)
	}
	popToolCommand(t, s, "response.create")
}
