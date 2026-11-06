package realtime

import (
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
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
