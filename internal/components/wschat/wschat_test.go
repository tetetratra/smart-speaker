package wschat

import (
	"testing"
	"time"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestMessageForEventSpeechStart(t *testing.T) {
	capturedAt := time.Date(2026, 7, 1, 12, 0, 0, 123, time.UTC)
	msg, targetID, ok := messageForEvent(types.Event{
		Kind: types.EventSpeechStart,
		Payload: types.SpeechEvent{
			Source:     "server-vad",
			CapturedAt: capturedAt,
		},
	})

	if !ok {
		t.Fatal("messageForEvent returned ok=false")
	}
	if targetID != "" {
		t.Fatalf("targetID = %q, want empty", targetID)
	}
	if got := msg["type"]; got != "speech_start" {
		t.Fatalf("type = %v, want speech_start", got)
	}
	if got := msg["source"]; got != "server-vad" {
		t.Fatalf("source = %v, want server-vad", got)
	}
	if got := msg["captured_at"]; got != capturedAt.Format(time.RFC3339Nano) {
		t.Fatalf("captured_at = %v, want %s", got, capturedAt.Format(time.RFC3339Nano))
	}
}

func TestMessageForAudioSentIncludesSegmentDuration(t *testing.T) {
	msg, _, ok := messageForEvent(types.Event{
		Kind:    types.EventAudioSent,
		Payload: types.AudioFrame{GenerationID: 2, SegmentID: "item/1", Text: "こんにちは", SegmentDuration: 1500 * time.Millisecond},
	})
	if !ok {
		t.Fatal("messageForEvent returned ok=false")
	}
	if got := msg["duration_ms"]; got != int64(1500) {
		t.Fatalf("duration_ms = %v, want 1500", got)
	}
	msg, _, ok = messageForEvent(types.Event{Kind: types.EventAudioSent, Payload: types.AudioFrame{GenerationID: 2, End: true}})
	if !ok {
		t.Fatal("messageForEvent returned ok=false")
	}
	if _, exists := msg["duration_ms"]; exists {
		t.Fatal("区間の長さが不明な通知に duration_ms を含めません")
	}
}

func TestMessageForEventWakeTranscription(t *testing.T) {
	msg, targetID, ok := messageForEvent(types.Event{
		Kind: types.EventWakeTranscription,
		Payload: types.WakeTranscription{
			PeerID:     "peer-1",
			Text:       "置きて",
			Outcome:    types.WakeTranscriptionJudged,
			Confidence: 0.4,
			Similarity: 0.67,
			Matched:    true,
			Error:      "STT停止",
		},
	})
	if !ok {
		t.Fatal("messageForEvent returned ok=false")
	}
	// 接続中のすべての画面へ送る。
	if targetID != "" {
		t.Fatalf("targetID = %q, want empty", targetID)
	}
	want := map[string]any{
		"type":       "wake_transcription",
		"peer_id":    "peer-1",
		"text":       "置きて",
		"outcome":    "judged",
		"confidence": 0.4,
		"similarity": 0.67,
		"matched":    true,
		"accepted":   false,
		"wake_only":  false,
		"awake":      false,
		"error":      "STT停止",
	}
	for key, value := range want {
		if msg[key] != value {
			t.Fatalf("%s = %v, want %v", key, msg[key], value)
		}
	}
	if len(msg) != len(want) {
		t.Fatalf("msg = %v, want %v", msg, want)
	}
}
