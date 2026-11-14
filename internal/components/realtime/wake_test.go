package realtime

import (
	"context"
	"encoding/json"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	waketool "github.com/tetetratra/smart-speaker/internal/tools/functions/wakemode"
	"github.com/tetetratra/smart-speaker/internal/types"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRequiredModeIgnoresExternalVADAndUsesLocalText(t *testing.T) {
	s, _ := wakemode.NewStore(filepath.Join(t.TempDir(), "mode.json"), wakemode.DefaultSettings())
	m, _ := s.SetRequired(true, time.Now())
	r := newRig(t, func(c *Config) { c.WakeMode = s })
	for _, kind := range []string{"input_audio_buffer.speech_started", "input_audio_buffer.speech_stopped", "input_audio_buffer.committed", "conversation.item.input_audio_transcription.completed"} {
		r.send(map[string]any{"type": kind, "item_id": "meeting", "transcript": "会議の話"})
	}
	r.wait("conversation.item.delete")
	if len(r.records()) != 0 {
		t.Fatal("呼びかけなしの音声を記録しました")
	}
	r.node.Upstream <- types.EventLocalWakeUtterance{LocalWakeUtterance: types.LocalWakeUtterance{PeerID: "test", Text: "起きて、今日の予定は", Revision: m.Revision, Confidence: 0.9}}
	item := r.wait("conversation.item.create")["item"].(map[string]any)
	content := item["content"].([]any)[0].(map[string]any)
	if content["type"] != "input_text" || content["text"] != "起きて、今日の予定は" {
		t.Fatal(item)
	}
	r.wait("response.create")
	records := r.records()
	if len(records) != 1 || records[0].Source != "local-stt" {
		t.Fatal(records)
	}
}

// 予約は内部指示なので呼びかけなしで実行でき、モード変更の返答も読み上げない。
func TestQuietScheduledToolDisablesRequiredMode(t *testing.T) {
	store, err := wakemode.NewStore(filepath.Join(t.TempDir(), "mode.json"), wakemode.DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetRequired(true, time.Now()); err != nil {
		t.Fatal(err)
	}
	inbound := make(chan types.ConversationInput, 1)
	r := newRigWithInbound(t, inbound, func(c *Config) { c.WakeMode = store })
	inbound <- types.ExternalInput(types.ConversationRecord{Source: "automation", Text: "呼びかけ必須モードを解除して。読み上げ不要"}, nil)
	request := r.wait("response.create")
	r.send(map[string]any{"type": "response.created", "response": map[string]any{"id": "job", "metadata": request["response"].(map[string]any)["metadata"]}})
	r.send(map[string]any{"type": "response.function_call_arguments.done", "response_id": "job", "call_id": "mode-call", "name": "set_wake_mode", "arguments": `{"required":false}`})
	req := r.event(types.EventToolRequest{}).(types.EventToolRequest).ToolRequest
	var args map[string]any
	if err = json.Unmarshal(req.Arguments, &args); err != nil {
		t.Fatal(err)
	}
	result, err := (&waketool.Tool{Store: store}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	output, _ := json.Marshal(result)
	record := types.ToolResultRecord{ToolCallID: req.ToolCallID, Name: req.Name, GenerationID: req.GenerationID, Output: output}
	r.node.Upstream <- types.EventConversationCommitRequest{ConversationCommitRequest: types.ConversationCommitRequest{ToolResult: &record}}
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
			if types.Is[types.EventResponseText](e) {
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

// 呼びかけだけの発話もRealtimeへ送り、その応答にだけ起きていることを伝える指示を付ける。
func TestWakeOnlyUtteranceAsksRealtimeForShortReply(t *testing.T) {
	s, _ := wakemode.NewStore(filepath.Join(t.TempDir(), "mode.json"), wakemode.DefaultSettings())
	m, _ := s.SetRequired(true, time.Now())
	r := newRig(t, func(c *Config) { c.WakeMode = s })
	r.node.Upstream <- types.EventLocalWakeUtterance{LocalWakeUtterance: types.LocalWakeUtterance{PeerID: "test", Text: "起きて", Revision: m.Revision, Confidence: 0.9, WakeOnly: true}}
	item := r.wait("conversation.item.create")["item"].(map[string]any)
	if text := item["content"].([]any)[0].(map[string]any)["text"]; text != "起きて" {
		t.Fatal(item)
	}
	request := r.wait("response.create")["response"].(map[string]any)
	if !strings.Contains(request["instructions"].(string), "呼びかけの言葉だけを言いました") {
		t.Fatal("呼びかけだけの発話への指示がありません", request["instructions"])
	}
	r.send(map[string]any{"type": "response.created", "response": map[string]any{"id": "ack", "metadata": request["metadata"]}})
	r.send(map[string]any{"type": "response.done", "response": map[string]any{"id": "ack", "status": "completed"}})
	// 本文のある次の発話には指示を持ち越さない。
	r.node.Upstream <- types.EventLocalWakeUtterance{LocalWakeUtterance: types.LocalWakeUtterance{PeerID: "test", Text: "起きて、天気は", Revision: m.Revision, Confidence: 0.9}}
	next := r.wait("response.create")["response"].(map[string]any)
	if strings.Contains(next["instructions"].(string), "呼びかけの言葉だけを言いました") {
		t.Fatal("本文のある発話に呼びかけだけの指示を付けました")
	}
}

// 呼びかけ後の会話中は、接続を作り直さずに音声とクラウドの発話検出を使い、終了時に未確定の音声を捨てる。
func TestConversationAfterWakeUsesAudioUntilItEnds(t *testing.T) {
	s, _ := wakemode.NewStore(filepath.Join(t.TempDir(), "mode.json"), wakemode.DefaultSettings())
	m, _ := s.SetRequired(true, time.Now())
	r := newRig(t, func(c *Config) { c.WakeMode = s })
	frame := types.EventRTCPeerAudioFrame{RTCPeerAudioFrame: types.RTCPeerAudioFrame{PeerID: "test", PCM: make([]byte, 1920), SampleRate: 48000, Channels: 1}}
	r.node.Upstream <- frame
	if d := s.Evaluate("起きて", 0.9, m.Revision, false, time.Now()); !d.Accepted {
		t.Fatal(d)
	}
	r.node.Upstream <- frame
	r.wait("input_audio_buffer.append")
	before := s.Snapshot(time.Now()).AwakeUntil
	time.Sleep(10 * time.Millisecond)
	r.utterance("followup")
	r.wait("response.create")
	if after := s.Snapshot(time.Now()).AwakeUntil; after == nil || !after.After(*before) {
		t.Fatal("会話中の発話で時間を数え直していません", before, after)
	}
	// 会話中が終わると接続は維持したまま、未確定の音声を捨てて音声を送らなくなる。
	if _, err := s.SetRequired(true, time.Now()); err != nil {
		t.Fatal(err)
	}
	r.node.Upstream <- frame
	r.wait("input_audio_buffer.clear")
	timer := time.NewTimer(80 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case c := <-r.commands:
			if c["type"] == "input_audio_buffer.append" || c["type"] == "session.update" {
				t.Fatal("会話の終了後に音声を送るか接続を作り直しました", c)
			}
		case conn := <-r.connections:
			t.Fatal("会話中の切替で再接続しました", conn)
		case <-timer.C:
			return
		}
	}
}

func TestLocalWakeInvalidatesOldAudioBeforeCancellationCompletes(t *testing.T) {
	store, err := wakemode.NewStore(filepath.Join(t.TempDir(), "mode.json"), wakemode.DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	mode, err := store.SetRequired(true, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s := newToolResponseStage(t)
	s.cfg.WakeMode = store
	s.peers = map[string]bool{"test": true}
	old := s.cfg.Generation.Current()
	s.active = "old-response"
	s.pendingOutput = map[string]bool{"old-item": true}
	s.items = map[string]*assistantItem{"old-item": {generation: old}}
	s.localWake(context.Background(), types.LocalWakeUtterance{PeerID: "test", Revision: mode.Revision, Text: "スピーカー、止めて"})
	popToolCommand(t, s, "response.cancel")
	if s.cfg.Generation.Current() <= old || s.hasPendingOutput() {
		t.Fatal("キャンセル完了前に旧音声が失効しませんでした")
	}
	var request types.Event
	for len(s.node.Downstream) > 0 {
		e := <-s.node.Downstream
		if types.Is[types.EventConversationInput](e) && e.(types.EventConversationInput).ConversationInput.ModelHandling == types.ModelHandlingAddAndRespond {
			request = e
		}
	}
	if !types.Is[types.EventConversationInput](request) {
		t.Fatal("応答依頼なし")
	}
	input := request.(types.EventConversationInput).ConversationInput
	if input.Record.GenerationID != s.cfg.Generation.Current() {
		t.Fatal(input)
	}
	generation := s.cfg.Generation.Current()
	s.active = ""
	s.handle(context.Background(), request)
	if s.cfg.Generation.Current() != generation {
		t.Fatal("会話管理から戻った入力が世代を二重更新しました")
	}
}
