package rtcpeer

import (
	"encoding/binary"
	"log"
	"time"

	"github.com/pion/webrtc/v4"
	opus "gopkg.in/hraban/opus.v2"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func (s *stage) handleIncomingTrack(peerID string, trackRemote *webrtc.TrackRemote) {
	codec := trackRemote.Codec()
	sampleRate := int(codec.ClockRate)
	if sampleRate <= 0 {
		sampleRate = webrtcSampleRate
	}
	channels := int(codec.Channels)
	if channels <= 0 {
		channels = 1
	}
	decoder, err := opus.NewDecoder(sampleRate, channels)
	if err != nil {
		log.Printf("rtcpeer: incoming opus decoder create error: %v", err)
		return
	}
	pcmBuf := make([]int16, 5760*max(1, channels))

	for {
		pkt, _, err := trackRemote.ReadRTP()
		if err != nil {
			log.Printf("rtcpeer: incoming audio read error: %v", err)
			return
		}
		nPerChannel, err := decoder.Decode(pkt.Payload, pcmBuf)
		if err != nil || nPerChannel <= 0 {
			continue
		}
		total := nPerChannel * max(1, channels)
		if total > len(pcmBuf) {
			continue
		}
		mono := downmixToMono(pcmBuf[:total], channels)
		if len(mono) == 0 {
			continue
		}
		s.emit(types.Event{
			Kind: types.EventRTCPeerAudioFrame,
			Payload: types.RTCPeerAudioFrame{
				PeerID:     peerID,
				Samples:    mono,
				PCM:        int16ToBytes(mono),
				SampleRate: sampleRate,
				Channels:   1,
				DurationMs: packetDurationMs(len(mono), sampleRate),
				CapturedAt: time.Now(),
			},
		})
	}
}

func downmixToMono(in []int16, channels int) []int16 {
	if len(in) == 0 {
		return nil
	}
	if channels <= 1 {
		out := make([]int16, len(in))
		copy(out, in)
		return out
	}
	frames := len(in) / channels
	if frames <= 0 {
		return nil
	}
	out := make([]int16, frames)
	for i := 0; i < frames; i++ {
		sum := 0
		for c := 0; c < channels; c++ {
			sum += int(in[i*channels+c])
		}
		out[i] = int16(sum / channels)
	}
	return out
}

func int16ToBytes(in []int16) []byte {
	if len(in) == 0 {
		return nil
	}
	out := make([]byte, len(in)*2)
	for i, v := range in {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(v))
	}
	return out
}

func packetDurationMs(sampleCount int, sampleRate int) int {
	if sampleCount <= 0 || sampleRate <= 0 {
		return 0
	}
	ms := sampleCount * 1000 / sampleRate
	if ms <= 0 {
		return 20
	}
	return ms
}
