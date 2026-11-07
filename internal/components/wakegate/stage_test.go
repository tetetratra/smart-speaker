package wakegate

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"github.com/tetetratra/smart-speaker/internal/types"
	"path/filepath"
	"testing"
	"time"
)

type fakeSTT struct {
	calls   chan []byte
	answers chan result
}

func (f *fakeSTT) Transcribe(ctx context.Context, pcm []byte, rate int) (Transcript, error) {
	select {
	case f.calls <- pcm:
	case <-ctx.Done():
		return Transcript{}, ctx.Err()
	}
	select {
	case r := <-f.answers:
		return r.transcript, r.err
	case <-ctx.Done():
		return Transcript{}, ctx.Err()
	}
}

type gateRig struct {
	ctx   context.Context
	node  *graph.Stage
	store *wakemode.Store
	stt   *fakeSTT
}

func newGate(t *testing.T, required bool) *gateRig {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	s, err := wakemode.NewStore(filepath.Join(t.TempDir(), "mode.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Set(required, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	f := &fakeSTT{calls: make(chan []byte, 8), answers: make(chan result, 8)}
	n := NewStage(Config{Store: s, STT: f})
	go n.Run(ctx)
	r := &gateRig{ctx, n, s, f}
	r.sink(t, true)
	return r
}
func (r *gateRig) sink(t *testing.T, connected bool) {
	t.Helper()
	select {
	case r.node.Upstream <- types.Event{Kind: types.EventRTCPeerOutputSink, Payload: types.RTCPeerOutputSink{PeerID: "a", Connected: connected}}:
	case <-r.ctx.Done():
		t.Fatal(r.ctx.Err())
	}
	select {
	case <-r.node.Downstream:
	case <-r.ctx.Done():
		t.Fatal(r.ctx.Err())
	}
}
func (r *gateRig) frame(t *testing.T, voiced bool) {
	t.Helper()
	pcm := make([]byte, 1600)
	if voiced {
		for i := 0; i < len(pcm); i += 2 {
			binary.LittleEndian.PutUint16(pcm[i:], 4000)
		}
	}
	select {
	case r.node.Upstream <- types.Event{Kind: types.EventRTCPeerAudioFrame, Payload: types.RTCPeerAudioFrame{PeerID: "a", PCM: pcm, SampleRate: 8000, Channels: 1}}:
	case <-r.ctx.Done():
		t.Fatal(r.ctx.Err())
	}
}
func (r *gateRig) utterance(t *testing.T) {
	for i := 0; i < 3; i++ {
		r.frame(t, true)
	}
	for i := 0; i < 8; i++ {
		r.frame(t, false)
	}
	select {
	case pcm := <-r.stt.calls:
		if len(pcm) == 0 {
			t.Fatal("空の音声")
		}
	case <-r.ctx.Done():
		t.Fatal("認識されませんでした")
	}
}
func (r *gateRig) noEvent(t *testing.T) {
	t.Helper()
	select {
	case e := <-r.node.Downstream:
		t.Fatal("未許可の出力", e)
	case <-time.After(80 * time.Millisecond):
	}
}
func TestDisabledPassesAudioWithoutSTT(t *testing.T) {
	r := newGate(t, false)
	go r.frame(t, true)
	select {
	case e := <-r.node.Downstream:
		if e.Kind != types.EventRTCPeerAudioFrame {
			t.Fatal(e)
		}
	case <-r.ctx.Done():
		t.Fatal(r.ctx.Err())
	}
	select {
	case <-r.stt.calls:
		t.Fatal("不要モードでSTTを呼びました")
	default:
	}
}
func (r *gateRig) wake(t *testing.T, text string) types.LocalWakeUtterance {
	t.Helper()
	select {
	case e := <-r.node.Downstream:
		p, ok := e.Payload.(types.LocalWakeUtterance)
		if !ok || p.Text != text || p.Confidence != 0.9 {
			t.Fatal(e)
		}
		return p
	case <-r.ctx.Done():
		t.Fatal(r.ctx.Err())
	}
	return types.LocalWakeUtterance{}
}
func TestUtterancesAreGatedUntilWakeAndThenPassRawAudio(t *testing.T) {
	r := newGate(t, true)
	// 呼びかけがない・文頭にない・低信頼度・認識失敗の発話は送らない。
	for _, answer := range []result{
		{transcript: Transcript{Text: "続きも教えて", Confidence: 0.9}},
		{transcript: Transcript{Text: "明日は早く家を出るので起きて", Confidence: 0.9}},
		{transcript: Transcript{Text: "起きて", Confidence: 0.1}},
		{err: errors.New("STT停止")},
	} {
		r.utterance(t)
		r.stt.answers <- answer
		r.noEvent(t)
	}
	r.utterance(t)
	r.stt.answers <- result{transcript: Transcript{Text: "ねえ、起きて。予定を教えて", Confidence: 0.9}}
	if p := r.wake(t, "ねえ、起きて。予定を教えて"); p.WakeOnly {
		t.Fatal("本文のある発話を呼びかけだけと判定しました")
	}
	// 会話中はローカルSTTを通さず、音声をそのまま送る。
	go r.frame(t, true)
	select {
	case e := <-r.node.Downstream:
		if e.Kind != types.EventRTCPeerAudioFrame {
			t.Fatal(e)
		}
	case <-r.ctx.Done():
		t.Fatal(r.ctx.Err())
	}
	select {
	case <-r.stt.calls:
		t.Fatal("会話中にSTTを呼びました")
	default:
	}
}
func TestUtteranceQueuedBeforeWakeIsAcceptedAfterWake(t *testing.T) {
	r := newGate(t, true)
	r.utterance(t)
	// 呼びかけの認識中に続けて話した発話も、呼びかけの確定後に受け付ける。
	for i := 0; i < 3; i++ {
		r.frame(t, true)
	}
	for i := 0; i < 8; i++ {
		r.frame(t, false)
	}
	r.stt.answers <- result{transcript: Transcript{Text: "起きて", Confidence: 0.9}}
	if p := r.wake(t, "起きて"); !p.WakeOnly {
		t.Fatal("呼びかけだけの発話を判定できません")
	}
	<-r.stt.calls
	r.stt.answers <- result{transcript: Transcript{Text: "今日の天気は", Confidence: 0.9}}
	r.wake(t, "今日の天気は")
}
func TestPendingRecognitionIsDiscardedAfterModeChangeOrReconnect(t *testing.T) {
	for _, change := range []string{"mode", "peer"} {
		t.Run(change, func(t *testing.T) {
			r := newGate(t, true)
			r.utterance(t)
			if change == "mode" {
				_, _ = r.store.Set(false, nil, time.Now())
			} else {
				r.sink(t, false)
				r.sink(t, true)
			}
			r.stt.answers <- result{transcript: Transcript{Text: "起きて、今何時", Confidence: 0.9}}
			r.noEvent(t)
		})
	}
}
