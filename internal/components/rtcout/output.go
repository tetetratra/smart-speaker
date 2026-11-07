package rtcout

import (
	"encoding/base64"
	"encoding/binary"
	"log"
	"math"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/tetetratra/smart-speaker/internal/types"
	opus "gopkg.in/hraban/opus.v2"
)

func (s *stage) sendFrame(frame types.AudioFrame) bool {
	if frame.End {
		return len(s.sinks) > 0
	}
	raw, err := base64.StdEncoding.DecodeString(frame.Audio)
	if err != nil || len(raw) == 0 || len(raw)%2 != 0 || len(raw) > 960 || frame.SampleRate != 24000 {
		return false
	}
	// 末尾の20ms未満の音声だけ無音で埋め、次の文章や世代へ持ち越さない。
	pcm := make([]int16, webrtcSampleRate*opusFrameMs/1000)
	copy(pcm, upsampleBy2(bytesToInt16(raw)))
	sent := false
	for _, sink := range s.sinks {
		samples := make([]int16, len(pcm))
		for i, sample := range pcm {
			samples[i] = int16(max(-32768, min(32767, int(float64(sample)*sink.gain))))
		}
		if sink.opusChannels == 2 {
			samples = upmixToStereo(samples)
		}
		packet := make([]byte, 4000)
		n, err := sink.encoder.Encode(samples, packet)
		if err != nil {
			log.Printf("rtcout: 音声圧縮失敗: %v", err)
			continue
		}
		if err := sink.writer.WriteSample(media.Sample{Data: packet[:n], Duration: opusFrameMs * time.Millisecond}); err != nil {
			log.Printf("rtcout: 音声送信失敗: %v", err)
			continue
		}
		sent = true
	}
	return sent
}

func (s *stage) handlePeerOutputSink(evt types.RTCPeerOutputSink) {
	if evt.PeerID == "" {
		return
	}
	if !evt.Connected || evt.Writer == nil {
		delete(s.sinks, evt.PeerID)
		delete(s.gains, evt.PeerID)
		return
	}
	channels := evt.OpusChannels
	if channels != 2 {
		channels = 1
	}
	encoder, err := opus.NewEncoder(webrtcSampleRate, channels, opus.AppVoIP)
	if err != nil {
		log.Printf("rtcout: opus encoder error: %v", err)
		return
	}
	gain, ok := s.gains[evt.PeerID]
	if !ok {
		gain = 1
	}
	s.sinks[evt.PeerID] = &peerSink{writer: evt.Writer, encoder: encoder, opusChannels: channels, gain: gain}
}

// ブラウザの音声をWeb Audioへ回さず、端末別の音量と1.5倍の増幅を送信前に適用する。
func (s *stage) handleVolume(sig types.RTCSignal) {
	if sig.Type != "webrtc.volume" || sig.Gain == nil || sig.ClientID == "" {
		return
	}
	gain := *sig.Gain
	if math.IsNaN(gain) || math.IsInf(gain, 0) || gain < 0 || gain > 1.5 {
		return
	}
	if s.gains == nil {
		s.gains = map[string]float64{}
	}
	s.gains[sig.ClientID] = gain
	if sink := s.sinks[sig.ClientID]; sink != nil {
		sink.gain = gain
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
