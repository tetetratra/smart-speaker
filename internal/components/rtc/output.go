package rtc

import (
	"encoding/base64"
	"encoding/binary"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"

	types "smart-speaker/internal/types"
)

func (s *stage) sendLoop() {
	ticker := time.NewTicker(time.Millisecond * opusFrameMs)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.sendOpusFrame()
		}
	}
}

func (s *stage) sendOpusFrame() {
	s.mu.Lock()
	peers := make([]*peerState, 0, len(s.peers))
	for _, peer := range s.peers {
		peers = append(peers, peer)
	}
	s.mu.Unlock()

	for _, peer := range peers {
		peer.mu.Lock()
		if peer.peer == nil || peer.track == nil || peer.encoder == nil || !peer.connected {
			peer.mu.Unlock()
			continue
		}
		frameSize := webrtcSampleRate * opusFrameMs / 1000 * max(1, peer.opusChannels)
		if len(peer.audioBuf) < frameSize {
			peer.mu.Unlock()
			continue
		}
		frame := make([]int16, frameSize)
		copy(frame, peer.audioBuf[:frameSize])
		peer.audioBuf = peer.audioBuf[frameSize:]
		track := peer.track
		encoder := peer.encoder
		peer.mu.Unlock()

		opusBuf := make([]byte, 4000)
		n, err := encoder.Encode(frame, opusBuf)
		if err != nil {
			continue
		}
		_ = track.WriteSample(media.Sample{Data: opusBuf[:n], Duration: time.Millisecond * opusFrameMs})
	}
}

func (s *stage) handleTTSAudio(audio types.OutputAudio) {
	raw, err := base64.StdEncoding.DecodeString(audio.Audio)
	if err != nil {
		return
	}
	pcm := bytesToInt16(raw)
	if len(pcm) == 0 {
		return
	}
	pcm = upsampleBy2(pcm)
	if len(pcm) == 0 {
		return
	}
	s.mu.Lock()
	peers := make([]*peerState, 0, len(s.peers))
	for _, peer := range s.peers {
		peers = append(peers, peer)
	}
	s.mu.Unlock()
	for _, peer := range peers {
		peer.mu.Lock()
		if peer.peer == nil || peer.track == nil || peer.encoder == nil || !peer.connected {
			peer.mu.Unlock()
			continue
		}
		localPCM := pcm
		if peer.opusChannels == 2 {
			localPCM = upmixToStereo(pcm)
		}
		peer.audioBuf = append(peer.audioBuf, localPCM...)
		peer.mu.Unlock()
	}
}

func (s *stage) handleTTSCancel() {
	s.mu.Lock()
	peers := make([]*peerState, 0, len(s.peers))
	for _, peer := range s.peers {
		peers = append(peers, peer)
	}
	s.mu.Unlock()
	for _, peer := range peers {
		peer.mu.Lock()
		peer.audioBuf = nil
		peer.mu.Unlock()
	}
}

func bytesToInt16(b []byte) []int16 {
	if len(b)%2 != 0 {
		return nil
	}
	out := make([]int16, len(b)/2)
	for i := 0; i < len(out); i++ {
		out[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
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

func upsampleBy2(in []int16) []int16 {
	if len(in) == 0 {
		return nil
	}
	out := make([]int16, len(in)*2)
	for i := 0; i < len(in); i++ {
		out[i*2] = in[i]
		if i == len(in)-1 {
			out[i*2+1] = in[i]
		} else {
			next := in[i+1]
			out[i*2+1] = int16((int(in[i]) + int(next)) / 2)
		}
	}
	return out
}

func upmixToStereo(in []int16) []int16 {
	if len(in) == 0 {
		return nil
	}
	out := make([]int16, len(in)*2)
	for i := 0; i < len(in); i++ {
		out[i*2] = in[i]
		out[i*2+1] = in[i]
	}
	return out
}
