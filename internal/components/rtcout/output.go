package rtcout

import (
	"encoding/base64"
	"encoding/binary"
	"log"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"
	opus "gopkg.in/hraban/opus.v2"

	types "github.com/tetetratra/smart-speaker/internal/types"
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
	sinks := make([]*peerSink, 0, len(s.sinks))
	for _, sink := range s.sinks {
		sinks = append(sinks, sink)
	}
	s.mu.Unlock()

	for _, sink := range sinks {
		sink.mu.Lock()
		if sink.writer == nil || sink.encoder == nil || !sink.connected {
			sink.mu.Unlock()
			continue
		}
		frameSize := webrtcSampleRate * opusFrameMs / 1000 * max(1, sink.opusChannels)
		if len(sink.audioBuf) < frameSize {
			sink.mu.Unlock()
			continue
		}
		frame := make([]int16, frameSize)
		copy(frame, sink.audioBuf[:frameSize])
		sink.audioBuf = sink.audioBuf[frameSize:]
		writer := sink.writer
		encoder := sink.encoder
		sink.mu.Unlock()

		opusBuf := make([]byte, 4000)
		n, err := encoder.Encode(frame, opusBuf)
		if err != nil {
			continue
		}
		_ = writer.WriteSample(media.Sample{Data: opusBuf[:n], Duration: time.Millisecond * opusFrameMs})
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
	sinks := make([]*peerSink, 0, len(s.sinks))
	for _, sink := range s.sinks {
		sinks = append(sinks, sink)
	}
	s.mu.Unlock()
	for _, sink := range sinks {
		sink.mu.Lock()
		if sink.writer == nil || sink.encoder == nil || !sink.connected {
			sink.mu.Unlock()
			continue
		}
		localPCM := pcm
		if sink.opusChannels == 2 {
			localPCM = upmixToStereo(pcm)
		}
		sink.audioBuf = append(sink.audioBuf, localPCM...)
		sink.mu.Unlock()
	}
}

func (s *stage) handlePeerOutputSink(evt types.RTCPeerOutputSink) {
	if evt.PeerID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !evt.Connected || evt.Writer == nil {
		delete(s.sinks, evt.PeerID)
		return
	}
	opusChannels := evt.OpusChannels
	if opusChannels <= 0 {
		opusChannels = 1
	}
	encoder, err := opus.NewEncoder(webrtcSampleRate, opusChannels, opus.AppVoIP)
	if err != nil {
		log.Printf("rtcout: opus encoder error: %v", err)
		return
	}
	s.sinks[evt.PeerID] = &peerSink{
		id:           evt.PeerID,
		writer:       evt.Writer,
		encoder:      encoder,
		connected:    true,
		opusChannels: opusChannels,
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
