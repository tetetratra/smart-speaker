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
	s, err := wakemode.NewStore(filepath.Join(t.TempDir(), "mode.json"), wakemode.DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRequired(required, time.Now()); err != nil {
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
	case r.node.Upstream <- types.EventRTCPeerOutputSink{RTCPeerOutputSink: types.RTCPeerOutputSink{PeerID: "a", Connected: connected}}:
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
	amplitude := uint16(0)
	if voiced {
		amplitude = 4000
	}
	r.frameAt(t, amplitude)
}

// frameAtは指定した振幅の100ms（8kHz）の音声を送る。
func (r *gateRig) frameAt(t *testing.T, amplitude uint16) {
	t.Helper()
	pcm := make([]byte, 1600)
	for i := 0; i < len(pcm); i += 2 {
		binary.LittleEndian.PutUint16(pcm[i:], amplitude)
	}
	select {
	case r.node.Upstream <- types.EventRTCPeerAudioFrame{RTCPeerAudioFrame: types.RTCPeerAudioFrame{PeerID: "a", PCM: pcm, SampleRate: 8000, Channels: 1}}:
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
		if !types.Is[types.EventRTCPeerAudioFrame](e) {
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

// transcribedは、画面のログへ送る認識結果を1件受け取る。
func (r *gateRig) transcribed(t *testing.T) types.WakeTranscription {
	t.Helper()
	select {
	case e := <-r.node.Downstream:
		pEvent, ok := e.(types.EventWakeTranscription)
		p := pEvent.WakeTranscription
		if !ok || !types.Is[types.EventWakeTranscription](e) || p.PeerID != "a" {
			t.Fatal(e)
		}
		return p
	case <-r.ctx.Done():
		t.Fatal(r.ctx.Err())
	}
	return types.WakeTranscription{}
}

// wakeは、受理した認識結果のログと、Realtimeへ渡す発話を順に受け取る。
func (r *gateRig) wake(t *testing.T, text string) types.LocalWakeUtterance {
	t.Helper()
	if tr := r.transcribed(t); tr.Text != text || tr.Outcome != types.WakeTranscriptionJudged || !tr.Accepted {
		t.Fatal("受理した認識結果のログが違います", tr)
	}
	select {
	case e := <-r.node.Downstream:
		pEvent, ok := e.(types.EventLocalWakeUtterance)
		p := pEvent.LocalWakeUtterance
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
	// 呼びかけがない・文頭にない・低信頼度・認識失敗の発話はRealtimeへ送らない。
	// 見逃しを調べるため、書き起こしと判定は画面のログへ送る。
	for _, answer := range []result{
		{transcript: Transcript{Text: "続きも教えて", Confidence: 0.9}},
		{transcript: Transcript{Text: "明日は早く家を出るので起きて", Confidence: 0.9}},
		{transcript: Transcript{Text: "起きて", Confidence: 0.1}},
		{err: errors.New("STT停止")},
	} {
		r.utterance(t)
		r.stt.answers <- answer
		tr := r.transcribed(t)
		if tr.Text != answer.transcript.Text || tr.Outcome != types.WakeTranscriptionJudged || tr.Accepted {
			t.Fatal("拒否した認識結果のログが違います", tr)
		}
		if answer.err != nil && tr.Error != answer.err.Error() {
			t.Fatal("STTの失敗内容がログにありません", tr)
		}
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
		if !types.Is[types.EventRTCPeerAudioFrame](e) {
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
				_, _ = r.store.SetRequired(false, time.Now())
			} else {
				r.sink(t, false)
				r.sink(t, true)
			}
			r.stt.answers <- result{transcript: Transcript{Text: "起きて、今何時", Confidence: 0.9}}
			// 捨てた認識結果も、理由とともに画面のログへ送る。
			want := types.WakeTranscriptionDiscardedModeChange
			if change == "peer" {
				want = types.WakeTranscriptionDiscardedReconnect
			}
			if tr := r.transcribed(t); tr.Text != "起きて、今何時" || tr.Outcome != want || tr.Accepted {
				t.Fatal("捨てた認識結果のログが違います", tr)
			}
			r.noEvent(t)
		})
	}
}

// 小さめの声の短い「起きて」（RMS約0.009・有声100ms）もSTTへ送り、それより小さい音は送らない。
func TestShortQuietUtteranceIsSentToSTT(t *testing.T) {
	r := newGate(t, true)
	// RMS約0.003はしきい値（0.005）未満なので発話とみなさない。
	r.frameAt(t, 100)
	for i := 0; i < 8; i++ {
		r.frame(t, false)
	}
	select {
	case <-r.stt.calls:
		t.Fatal("しきい値未満の音をSTTへ送りました")
	case <-time.After(80 * time.Millisecond):
	}
	r.frameAt(t, 300)
	for i := 0; i < 8; i++ {
		r.frame(t, false)
	}
	select {
	case <-r.stt.calls:
	case <-r.ctx.Done():
		t.Fatal("短い小さめの発話をSTTへ送りませんでした")
	}
}
