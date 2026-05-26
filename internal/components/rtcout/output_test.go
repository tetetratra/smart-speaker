package rtcout

import (
	"encoding/base64"
	"testing"

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
	s := &stage{sinks: map[string]*peerSink{}}
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

func TestHandleTTSAudioAppendsPCMToConnectedSinks(t *testing.T) {
	s := &stage{sinks: map[string]*peerSink{}}
	s.handlePeerOutputSink(types.RTCPeerOutputSink{
		PeerID:       "mono",
		Writer:       &sampleWriter{},
		Connected:    true,
		OpusChannels: 1,
	})
	s.handlePeerOutputSink(types.RTCPeerOutputSink{
		PeerID:       "stereo",
		Writer:       &sampleWriter{},
		Connected:    true,
		OpusChannels: 2,
	})

	pcm := int16ToBytes([]int16{10, 30})
	s.handleTTSAudio(types.OutputAudio{Audio: base64.StdEncoding.EncodeToString(pcm)})

	mono := s.sinks["mono"].audioBuf
	if want := []int16{10, 20, 30, 30}; !equalInt16(mono, want) {
		t.Fatalf("mono audioBuf = %#v, want %#v", mono, want)
	}
	stereo := s.sinks["stereo"].audioBuf
	if want := []int16{10, 10, 20, 20, 30, 30, 30, 30}; !equalInt16(stereo, want) {
		t.Fatalf("stereo audioBuf = %#v, want %#v", stereo, want)
	}
}

func TestHandleTTSAudioIgnoresInvalidAudio(t *testing.T) {
	s := &stage{sinks: map[string]*peerSink{}}
	s.handlePeerOutputSink(types.RTCPeerOutputSink{
		PeerID:    "peer-1",
		Writer:    &sampleWriter{},
		Connected: true,
	})

	s.handleTTSAudio(types.OutputAudio{Audio: "not-base64"})

	if got := s.sinks["peer-1"].audioBuf; len(got) != 0 {
		t.Fatalf("expected no buffered audio, got %#v", got)
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
