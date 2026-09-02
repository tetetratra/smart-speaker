package openaistt

import (
	"context"
	"testing"
	"time"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestStageSendsAudioAndCommit(t *testing.T) {
	conn := &fakeRealtimeConn{}
	st, err := NewStage(Config{
		APIKey: "openai",
		Model:  "gpt-realtime-whisper",
		dialer: fakeDialer{conn: conn},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)

	st.Upstream <- types.Event{Kind: types.EventRTCSpeechAudio, Payload: types.RTCSpeechAudio{
		Type:       types.RTCSpeechAudioStart,
		Prebuffer:  encodePCM16([]int16{100, 200}),
		SampleRate: openAIInputSampleRate,
		Channels:   1,
	}}
	st.Upstream <- types.Event{Kind: types.EventRTCSpeechAudio, Payload: types.RTCSpeechAudio{
		Type: types.RTCSpeechAudioFrame,
		PCM:  encodePCM16([]int16{300, 400}),
	}}
	st.Upstream <- types.Event{Kind: types.EventRTCSpeechAudio, Payload: types.RTCSpeechAudio{
		Type: types.RTCSpeechAudioEnd,
	}}

	if !waitUntil(func() bool { return conn.writeCount() >= 4 }) {
		t.Fatalf("writes = %d, want at least session.update, prebuffer append, frame append, commit", conn.writeCount())
	}
}

func TestConsumeRealtimeEventsEmitsInterimAndFinalTranscripts(t *testing.T) {
	conn := &fakeRealtimeConn{
		reads: [][]byte{
			[]byte(`{"type":"conversation.item.input_audio_transcription.delta","item_id":"item_1","delta":"明日"}`),
			[]byte(`{"type":"conversation.item.input_audio_transcription.delta","item_id":"item_1","delta":"の予定"}`),
			[]byte(`{"type":"conversation.item.input_audio_transcription.completed","item_id":"item_1","transcript":"明日の予定"}`),
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &stage{
		ctx:        ctx,
		downstream: make(chan types.Event, 3),
		session: &speechSession{
			ctx:    ctx,
			cancel: cancel,
			conn:   conn,
		},
	}

	s.consumeRealtimeEvents(s.session)

	interim := expectEvent(t, s.downstream)
	if interim.Kind != types.EventHumanInterimUtterance {
		t.Fatalf("first Kind = %s", interim.Kind)
	}
	if line := interim.Payload.(types.OutputLine); line.Text != "明日" || line.Final {
		t.Fatalf("first line = %#v", line)
	}
	interim = expectEvent(t, s.downstream)
	if line := interim.Payload.(types.OutputLine); line.Text != "明日の予定" || line.Final {
		t.Fatalf("second line = %#v", line)
	}
	final := expectEvent(t, s.downstream)
	if final.Kind != types.EventHumanUtterance {
		t.Fatalf("final Kind = %s", final.Kind)
	}
	if line := final.Payload.(types.OutputLine); line.Text != "明日の予定" || !line.Final {
		t.Fatalf("final line = %#v", line)
	}
}

func TestNewStageRequiresAPIKey(t *testing.T) {
	if _, err := NewStage(Config{}); err == nil {
		t.Fatal("expected error")
	}
}

type fakeDialer struct {
	conn realtimeConn
}

func (f fakeDialer) Dial(context.Context, Config) (realtimeConn, error) {
	return f.conn, nil
}

func expectEvent(t *testing.T, ch <-chan types.Event) types.Event {
	t.Helper()
	select {
	case evt := <-ch:
		return evt
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return types.Event{}
	}
}

func waitUntil(ok func() bool) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return ok()
}
