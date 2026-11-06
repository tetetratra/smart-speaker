package realtime

import (
	"encoding/json"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	waketool "github.com/tetetratra/smart-speaker/internal/tools/functions/wakemode"
	"github.com/tetetratra/smart-speaker/internal/types"
	"path/filepath"
	"testing"
	"time"
)

func TestRequiredModeIgnoresExternalVADAndUsesLocalText(t *testing.T) {
	s, _ := wakemode.NewStore(filepath.Join(t.TempDir(), "mode.json"))
	m, _ := s.Set(true, "", time.Now())
	r := newRig(t, func(c *Config) { c.WakeMode = s })
	for _, kind := range []string{"input_audio_buffer.speech_started", "input_audio_buffer.speech_stopped", "input_audio_buffer.committed", "conversation.item.input_audio_transcription.completed"} {
		r.send(map[string]any{"type": kind, "item_id": "meeting", "transcript": "会議の話"})
	}
	r.wait("conversation.item.delete")
	if len(r.history.Snapshot()) != 0 {
		t.Fatal("呼びかけなしの音声を記録しました")
	}
	r.node.Upstream <- types.Event{Kind: types.EventLocalWakeUtterance, Payload: types.LocalWakeUtterance{PeerID: "test", Text: "スピーカー、今日の予定は", Revision: m.Revision, Confidence: 0.9, Similarity: 1}}
	item := r.wait("conversation.item.create")["item"].(map[string]any)
	content := item["content"].([]any)[0].(map[string]any)
	if content["type"] != "input_text" || content["text"] != "スピーカー、今日の予定は" {
		t.Fatal(item)
	}
	r.wait("response.create")
	records := r.history.Snapshot()
	if len(records) != 1 || records[0].Source != "local-stt" {
		t.Fatal(records)
	}
}

// 予約は内部指示なので呼びかけなしで実行でき、モード変更の返答も読み上げない。
func TestQuietScheduledToolDisablesRequiredMode(t *testing.T) {
	store, err := wakemode.NewStore(filepath.Join(t.TempDir(), "mode.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Set(true, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	scheduled := make(chan types.ScheduledPrompt, 1)
	r := newRig(t, func(c *Config) { c.WakeMode = store; c.Scheduled = scheduled })
	scheduled <- types.ScheduledPrompt{ID: "end-work", Name: "仕事終了", Instruction: "呼びかけ必須モードを解除して。読み上げ不要", Speak: false}
	request := r.wait("response.create")
	r.send(map[string]any{"type": "response.created", "response": map[string]any{"id": "job", "metadata": request["response"].(map[string]any)["metadata"]}})
	r.send(map[string]any{"type": "response.function_call_arguments.done", "response_id": "job", "call_id": "mode-call", "name": "set_wake_mode", "arguments": `{"required":false}`})
	req := r.event(types.EventToolRequest).Payload.(types.ToolRequest)
	var args map[string]any
	if err = json.Unmarshal(req.Arguments, &args); err != nil {
		t.Fatal(err)
	}
	result, err := (&waketool.Tool{Store: store}).Run(args)
	if err != nil {
		t.Fatal(err)
	}
	output, _ := json.Marshal(result)
	record := types.ToolResultRecord{ToolCallID: req.ToolCallID, Name: req.Name, GenerationID: req.GenerationID, Output: output}
	r.node.Upstream <- types.Event{Payload: types.ConversationCommitRequest{ToolResult: &record}}
	r.send(map[string]any{"type": "response.done", "response": map[string]any{"id": "job", "status": "completed"}})
	update := r.wait("session.update")
	select {
	case r.conn = <-r.connections:
	case <-r.ctx.Done():
		t.Fatal(r.ctx.Err())
	}
	input := update["session"].(map[string]any)["audio"].(map[string]any)["input"].(map[string]any)
	if input["turn_detection"].(map[string]any)["interrupt_response"] != true || store.Snapshot(time.Now()).Required {
		t.Fatal("予約によるモード解除が反映されませんでした")
	}
	r.send(map[string]any{"type": "session.updated"})
	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()
check:
	for {
		select {
		case e := <-r.events:
			if e.Kind == types.EventResponseText {
				t.Fatal("無音の予約が音声合成へ届きました")
			}
		case <-timer.C:
			break check
		}
	}
	// 解除後は呼びかけのない通常の発話で応答を開始できる。
	r.utterance("ordinary")
	r.wait("response.create")
}
