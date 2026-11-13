package realtime

import (
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/types"
)

func startWait(t *testing.T, r *rig) map[string]any {
	t.Helper()
	r.utterance("user1")
	request := r.wait("response.create")
	r.send(map[string]any{"type": "response.created", "response": map[string]any{"id": "waiting", "metadata": request["response"].(map[string]any)["metadata"]}})
	r.send(map[string]any{"type": "response.function_call_arguments.done", "response_id": "waiting", "call_id": "wait1", "name": waitToolName, "arguments": "{}"})
	output := r.wait("conversation.item.create")["item"].(map[string]any)
	if output["type"] != "function_call_output" || output["call_id"] != "wait1" || output["output"] != `{"waiting":true}` {
		t.Fatalf("待機の完了結果が違います: %v", output)
	}
	return request
}

func TestWaitStaysSilentSkipsWhiteboardAndAcceptsNextUser(t *testing.T) {
	r := newRig(t, func(cfg *Config) {
		// 誤って読み取りに分類されても、待機は一般のツール・表示処理へ渡さない。
		cfg.Tools = []any{map[string]any{"type": "function", "name": "set_whiteboard", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}}}
	})
	startWait(t, r)
	r.send(map[string]any{"type": "response.done", "response": map[string]any{"id": "waiting", "status": "completed"}})
	// 同じ呼び出しの再配信でも、完了を二重に送ったり一般のツールを実行したりしない。
	r.send(map[string]any{"type": "response.function_call_arguments.done", "response_id": "waiting", "call_id": "wait1", "name": waitToolName, "arguments": "{}"})
	timer := time.NewTimer(80 * time.Millisecond)
	defer timer.Stop()
observe:
	for {
		select {
		case command := <-r.commands:
			t.Fatalf("待機後に不要なAPI要求が出ました: %v", command)
		case event := <-r.events:
			if event.Kind == types.EventToolRequest || event.Kind == types.EventResponseText {
				t.Fatalf("待機がツール実行・発話・画面更新を起動しました: %v", event)
			}
		case <-timer.C:
			break observe
		}
	}
	results := 0
	for _, record := range r.records() {
		if record.Source == waitToolName && record.Role == types.RoleToolResult {
			results++
		}
	}
	if results != 1 {
		t.Fatalf("待機の完了記録は一度だけ必要です: %d", results)
	}
	r.utterance("user2")
	next := r.wait("response.create")
	response := next["response"].(map[string]any)
	if response["tool_choice"] != nil {
		t.Fatal("待機が次の発話にホワイトボード更新を強制しました", next)
	}
	r.response("next", "answer", "こんにちは。", response["metadata"])
	if text := r.event(types.EventResponseText).Payload.(types.ResponseText).Text; text != "こんにちは。" {
		t.Fatal("待機後の通常応答が届きません", text)
	}
}

func TestWaitDoesNotSuppressUserCommittedBeforeResponseDone(t *testing.T) {
	r := newRig(t)
	startWait(t, r)
	// 待機を選んだ応答の終了通知より先に、次のユーザー発話が確定する順序を再現する。
	r.utterance("user2")
	r.send(map[string]any{"type": "response.done", "response": map[string]any{"id": "waiting"}})
	next := r.wait("response.create")
	if next["response"].(map[string]any)["metadata"].(map[string]any)["generation"] != "2" {
		t.Fatalf("待機が次の発話を抑制しました: %v", next)
	}
}

func TestServerVADCanBeSelectedForComparison(t *testing.T) {
	newRig(t, func(cfg *Config) { cfg.TurnDetection = "server_vad" })
}

func TestInvalidTurnDetectionFailsBeforeConnecting(t *testing.T) {
	if _, err := NewStage(Config{APIKey: "local", TurnDetection: "invalid"}); err == nil {
		t.Fatal("未対応の発話区切り設定が受理されました")
	}
}
