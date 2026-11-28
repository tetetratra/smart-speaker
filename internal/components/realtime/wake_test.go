package realtime

import (
	"context"
	"github.com/tetetratra/smart-speaker/internal/features/wakemode"
	"github.com/tetetratra/smart-speaker/internal/types"
	"path/filepath"
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
	before := r.gen.Current()
	r.node.Upstream <- types.EventLocalWakeUtterance{LocalWakeUtterance: types.LocalWakeUtterance{PeerID: "test", Text: "起きて、今日の予定は", Revision: m.Revision, Confidence: 0.9}}
	item := r.wait("conversation.item.create")["item"].(map[string]any)
	content := item["content"].([]any)[0].(map[string]any)
	if content["type"] != "input_text" || content["text"] != "起きて、今日の予定は" {
		t.Fatal(item)
	}
	// 応答は会話の流れの判定を待ってから始める。
	select {
	case cmd := <-r.commands:
		t.Fatal("判定の前に応答を始めました", cmd)
	case <-time.After(30 * time.Millisecond):
	}
	r.respond(before, "")
	r.wait("response.create")
	records := r.records()
	if len(records) != 1 || records[0].Source != "realtime" || records[0].Role != types.RoleUser {
		t.Fatal(records)
	}
}

// 頭脳AIのツールで呼びかけ必須モードが解除されたら、接続を作り直して、呼びかけのない発話にも応答できるようにする。
func TestRequiredModeChangedOutsideIsAppliedToSession(t *testing.T) {
	store, err := wakemode.NewStore(filepath.Join(t.TempDir(), "mode.json"), wakemode.DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetRequired(true, time.Now()); err != nil {
		t.Fatal(err)
	}
	r := newRig(t, func(c *Config) { c.WakeMode = store })
	if _, err = store.SetRequired(false, time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case r.conn = <-r.connections:
	case <-r.ctx.Done():
		t.Fatal(r.ctx.Err())
	}
	update := r.wait("session.update")
	input := update["session"].(map[string]any)["audio"].(map[string]any)["input"].(map[string]any)
	if input["turn_detection"].(map[string]any)["interrupt_response"] != true {
		t.Fatal("モード解除が反映されませんでした")
	}
	r.send(map[string]any{"type": "session.updated"})
	// 解除後は呼びかけのない通常の発話で応答を開始できる。
	r.utterance("ordinary")
	r.wait("response.create")
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
	s := newTestStage(t)
	s.cfg.WakeMode = store
	s.peers = map[string]bool{"test": true}
	old := s.cfg.Generation.Current()
	s.active = "old-response"
	s.pendingOutput = map[string]bool{"old-item": true}
	s.items = map[string]*assistantItem{"old-item": {generation: old}}
	s.localWake(context.Background(), types.LocalWakeUtterance{PeerID: "test", Revision: mode.Revision, Text: "スピーカー、止めて"})
	popCommand(t, s, "response.cancel")
	if s.cfg.Generation.Current() <= old || s.hasPendingOutput() {
		t.Fatal("キャンセル完了前に旧音声が失効しませんでした")
	}
	// 発話はユーザーの記録として出し、会話の流れが振り分けを頼めるようにする（応答はまだ始めない）。
	var record *types.ConversationInput
	for len(s.transcription.Downstream) > 0 {
		if e, ok := (<-s.transcription.Downstream).(types.EventConversationInput); ok {
			record = &e.ConversationInput
		}
	}
	if record == nil || !record.ModelAlreadyKnows || record.ModelShouldRespond || record.UserAlreadyKnows || record.Record.Role != types.RoleUser {
		t.Fatal("ユーザーの記録がありません", record)
	}
	if record.Record.GenerationID != s.cfg.Generation.Current() {
		t.Fatal(record)
	}
}
