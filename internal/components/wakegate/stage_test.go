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
	if _, err := s.Set(required, "", time.Now()); err != nil {
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
func TestEachUtteranceIsGatedAndRawAudioNeverLeaves(t *testing.T) {
	r := newGate(t, true)
	r.utterance(t)
	r.stt.answers <- result{transcript: Transcript{Text: "スビーカー、予定を教えて", Confidence: 0.9}}
	select {
	case e := <-r.node.Downstream:
		p, ok := e.Payload.(types.LocalWakeUtterance)
		if !ok || (p.Similarity < 0.79 || p.Similarity >= 1) || p.Confidence != 0.9 {
			t.Fatal(e)
		}
	case <-r.ctx.Done():
		t.Fatal(r.ctx.Err())
	}
	// 直前に呼びかけ済みでも次の発話は必須。
	r.utterance(t)
	r.stt.answers <- result{transcript: Transcript{Text: "続きも教えて", Confidence: 0.9}}
	r.noEvent(t)
	r.utterance(t)
	r.stt.answers <- result{transcript: Transcript{Text: "スピーカー", Confidence: 0.1}}
	r.noEvent(t)
	r.utterance(t)
	r.stt.answers <- result{err: errors.New("STT停止")}
	r.noEvent(t)
}
func TestPendingRecognitionIsDiscardedAfterModeChangeOrReconnect(t *testing.T) {
	for _, change := range []string{"mode", "peer"} {
		t.Run(change, func(t *testing.T) {
			r := newGate(t, true)
			r.utterance(t)
			if change == "mode" {
				_, _ = r.store.Set(false, "", time.Now())
			} else {
				r.sink(t, false)
				r.sink(t, true)
			}
			r.stt.answers <- result{transcript: Transcript{Text: "スピーカー、今何時", Confidence: 0.9}}
			r.noEvent(t)
		})
	}
}
