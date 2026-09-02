package openaistt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"testing"

	"nhooyr.io/websocket"
)

func TestSendSessionUpdateUsesTranscriptionConfig(t *testing.T) {
	conn := &fakeRealtimeConn{}

	err := sendSessionUpdate(context.Background(), conn, Config{
		Model:   "gpt-realtime-whisper",
		Phrases: []string{"スマートスピーカー", "bad<keyword", "AC-42"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var evt map[string]any
	if err := json.Unmarshal(conn.writes[0], &evt); err != nil {
		t.Fatal(err)
	}
	if evt["type"] != "session.update" {
		t.Fatalf("type = %v", evt["type"])
	}
	session := evt["session"].(map[string]any)
	audio := session["audio"].(map[string]any)
	input := audio["input"].(map[string]any)
	format := input["format"].(map[string]any)
	if format["type"] != "audio/pcm" || format["rate"].(float64) != openAIInputSampleRate {
		t.Fatalf("format = %#v", format)
	}
	transcription := input["transcription"].(map[string]any)
	if transcription["model"] != "gpt-realtime-whisper" {
		t.Fatalf("model = %v", transcription["model"])
	}
	keywords := transcription["keywords"].([]any)
	if len(keywords) != 2 || keywords[0] != "スマートスピーカー" || keywords[1] != "AC-42" {
		t.Fatalf("keywords = %#v", keywords)
	}
	if input["turn_detection"] != nil {
		t.Fatalf("turn_detection = %#v, want nil", input["turn_detection"])
	}
}

func TestSendAudioAppendChunksAndBase64EncodesPCM(t *testing.T) {
	conn := &fakeRealtimeConn{}
	pcm := make([]byte, openAIChunkBytes+3)
	for i := range pcm {
		pcm[i] = byte(i % 251)
	}

	if err := sendAudioAppend(context.Background(), conn, pcm); err != nil {
		t.Fatal(err)
	}

	if len(conn.writes) != 2 {
		t.Fatalf("writes = %d, want 2", len(conn.writes))
	}
	var first map[string]string
	if err := json.Unmarshal(conn.writes[0], &first); err != nil {
		t.Fatal(err)
	}
	if first["type"] != "input_audio_buffer.append" {
		t.Fatalf("type = %q", first["type"])
	}
	decoded, err := base64.StdEncoding.DecodeString(first["audio"])
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != openAIChunkBytes {
		t.Fatalf("first audio bytes = %d, want %d", len(decoded), openAIChunkBytes)
	}
	var second map[string]string
	if err := json.Unmarshal(conn.writes[1], &second); err != nil {
		t.Fatal(err)
	}
	decoded, err = base64.StdEncoding.DecodeString(second["audio"])
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 2 {
		t.Fatalf("second audio bytes = %d, want 2", len(decoded))
	}
}

func TestSendAudioCommit(t *testing.T) {
	conn := &fakeRealtimeConn{}

	if err := sendAudioCommit(context.Background(), conn); err != nil {
		t.Fatal(err)
	}

	var evt map[string]string
	if err := json.Unmarshal(conn.writes[0], &evt); err != nil {
		t.Fatal(err)
	}
	if evt["type"] != "input_audio_buffer.commit" {
		t.Fatalf("type = %q", evt["type"])
	}
}

type fakeRealtimeConn struct {
	mu     sync.Mutex
	writes [][]byte
	reads  [][]byte
	closed bool
}

func (f *fakeRealtimeConn) Write(_ context.Context, _ websocket.MessageType, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	copied := append([]byte(nil), data...)
	f.writes = append(f.writes, copied)
	return nil
}

func (f *fakeRealtimeConn) Read(_ context.Context) (websocket.MessageType, []byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.reads) == 0 {
		return websocket.MessageText, nil, context.Canceled
	}
	data := f.reads[0]
	f.reads = f.reads[1:]
	return websocket.MessageText, data, nil
}

func (f *fakeRealtimeConn) Close(websocket.StatusCode, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeRealtimeConn) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.writes)
}
