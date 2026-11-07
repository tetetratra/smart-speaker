package rtcout

import (
	"encoding/base64"
	"errors"
	opus "gopkg.in/hraban/opus.v2"
	"testing"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

type sampleWriter struct {
	samples []media.Sample
}

func (w *sampleWriter) WriteSample(sample media.Sample) error {
	w.samples = append(w.samples, sample)
	return nil
}

func TestBytesToInt16(t *testing.T) {
	got := bytesToInt16([]byte{0x01, 0x00, 0xfe, 0xff})
	want := []int16{1, -2}
	if !equalInt16(got, want) {
		t.Fatalf("bytesToInt16() = %#v, want %#v", got, want)
	}

	if got := bytesToInt16([]byte{0x01}); got != nil {
		t.Fatalf("bytesToInt16(odd) = %#v, want nil", got)
	}
}

func TestUpsampleBy2(t *testing.T) {
	got := upsampleBy2([]int16{10, 30, 50})
	want := []int16{10, 20, 30, 40, 50, 50}
	if !equalInt16(got, want) {
		t.Fatalf("upsampleBy2() = %#v, want %#v", got, want)
	}
}

func TestUpmixToStereo(t *testing.T) {
	got := upmixToStereo([]int16{10, -20})
	want := []int16{10, 10, -20, -20}
	if !equalInt16(got, want) {
		t.Fatalf("upmixToStereo() = %#v, want %#v", got, want)
	}
}

func TestHandlePeerOutputSinkAddsAndRemovesSink(t *testing.T) {
	s := &stage{sinks: map[string]*peerSink{}, gains: map[string]float64{}}
	writer := &sampleWriter{}

	s.handlePeerOutputSink(types.RTCPeerOutputSink{
		PeerID:    "peer-1",
		Writer:    writer,
		Connected: true,
	})

	sink := s.sinks["peer-1"]
	if sink == nil {
		t.Fatal("expected sink to be registered")
	}
	if sink.writer != writer {
		t.Fatal("expected writer to be stored")
	}
	if sink.opusChannels != 1 {
		t.Fatalf("expected default opus channels 1, got %d", sink.opusChannels)
	}
	if sink.encoder == nil {
		t.Fatal("expected opus encoder to be created")
	}

	s.handlePeerOutputSink(types.RTCPeerOutputSink{PeerID: "peer-1"})
	if _, ok := s.sinks["peer-1"]; ok {
		t.Fatal("expected disconnected sink to be removed")
	}
}

func TestSendFrameEncodesOnePacketPerPeerWithoutBuffering(t *testing.T) {
	s := &stage{sinks: map[string]*peerSink{}, gains: map[string]float64{}}
	for _, channels := range []int{1, 2} {
		writer := &sampleWriter{}
		s.handlePeerOutputSink(types.RTCPeerOutputSink{PeerID: "peer", Writer: writer, Connected: true, OpusChannels: channels})
		frame := types.AudioFrame{SampleRate: 24000, Audio: base64.StdEncoding.EncodeToString(make([]byte, 960))}
		if !s.sendFrame(frame) || len(writer.samples) != 1 {
			t.Fatal("一フレームを即送信できなかった")
		}
		decoder, err := opus.NewDecoder(48000, channels)
		if err != nil {
			t.Fatal(err)
		}
		n, err := decoder.Decode(writer.samples[0].Data, make([]int16, 960*channels))
		if err != nil || n != 960 || writer.samples[0].Duration != 20*time.Millisecond {
			t.Fatalf("20msのOpus音声ではない: %d %v", n, err)
		}
		// 端数を次の文章へ蓄積せず、独立した20msのパケットとして送る。
		frame.Audio = base64.StdEncoding.EncodeToString([]byte{1, 0})
		if !s.sendFrame(frame) || len(writer.samples) != 2 {
			t.Fatal("末尾の音声を送れなかった")
		}
	}
}

func TestInvalidFrameAndFailedWriteDoNotCountAsSent(t *testing.T) {
	s := &stage{sinks: map[string]*peerSink{}}
	writer := &sampleWriter{}
	s.handlePeerOutputSink(types.RTCPeerOutputSink{PeerID: "peer", Writer: writer, Connected: true})
	for _, frame := range []types.AudioFrame{
		{SampleRate: 24000, Audio: "not-base64"},
		{SampleRate: 24000, Audio: base64.StdEncoding.EncodeToString([]byte{1})},
		{SampleRate: 48000, Audio: base64.StdEncoding.EncodeToString(make([]byte, 960))},
		{SampleRate: 24000, Audio: base64.StdEncoding.EncodeToString(make([]byte, 1920))},
	} {
		if s.sendFrame(frame) {
			t.Fatal("不正な音声を送信済みにした")
		}
	}
	if len(writer.samples) != 0 {
		t.Fatal("不正な音声が送信された")
	}
	s.sinks["peer"].writer = failingWriter{}
	if s.sendFrame(types.AudioFrame{SampleRate: 24000, Audio: base64.StdEncoding.EncodeToString(make([]byte, 960))}) {
		t.Fatal("書き込み失敗を送信済みにした")
	}
}

type failingWriter struct{}

func (failingWriter) WriteSample(media.Sample) error { return errors.New("送信失敗") }

func TestVolumeIsAppliedPerPeerIncludingBeforeConnection(t *testing.T) {
	s := &stage{sinks: map[string]*peerSink{}}
	gain := 1.5
	s.handleVolume(types.RTCSignal{Type: "webrtc.volume", ClientID: "a", Gain: &gain})
	s.handlePeerOutputSink(types.RTCPeerOutputSink{PeerID: "a", Writer: &sampleWriter{}, Connected: true})
	s.handlePeerOutputSink(types.RTCPeerOutputSink{PeerID: "b", Writer: &sampleWriter{}, Connected: true})
	if s.sinks["a"].gain != 1.5 || s.sinks["b"].gain != 1 {
		t.Fatal("端末別の音量が保持されなかった")
	}
	gain = 0
	s.handleVolume(types.RTCSignal{Type: "webrtc.volume", ClientID: "a", Gain: &gain})
	if s.sinks["a"].gain != 0 {
		t.Fatal("ミュートが反映されなかった")
	}
	gain = 100
	s.handleVolume(types.RTCSignal{Type: "webrtc.volume", ClientID: "a", Gain: &gain})
	if s.sinks["a"].gain != 0 {
		t.Fatal("範囲外の増幅が反映された")
	}
}

func equalInt16(a, b []int16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
