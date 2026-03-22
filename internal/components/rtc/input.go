package rtc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	speech "cloud.google.com/go/speech/apiv2"
	speechpb "cloud.google.com/go/speech/apiv2/speechpb"
	"github.com/pion/webrtc/v4"
	"google.golang.org/api/option"
	opus "gopkg.in/hraban/opus.v2"

	types "smart-speaker/internal/types"
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
		log.Printf("rtc: incoming opus decoder create error: %v", err)
		return
	}
	pcmBuf := make([]int16, 5760*max(1, channels))

	peerState := s.getPeer(peerID)
	if peerState == nil {
		return
	}
	peerState.mu.Lock()
	peerState.inputSampleRate = sampleRate
	peerState.prebuffer = newPCMRingBuffer(prebufferBytes(sampleRate, 1, prebufferSeconds))
	peerState.speechActive = false
	peerState.voicedMs = 0
	peerState.silenceMs = 0
	peerState.backgroundEnergies = nil
	peerState.speechThreshold = adaptiveVADMinThreshold
	peerState.speechThresholdUpdatedAt = time.Time{}
	peerState.mu.Unlock()

	for {
		pkt, _, err := trackRemote.ReadRTP()
		if err != nil {
			log.Printf("rtc: incoming audio read error: %v", err)
			peerState.mu.Lock()
			peerState.speechActive = false
			peerState.voicedMs = 0
			peerState.silenceMs = 0
			peerState.mu.Unlock()
			s.clearActiveSpeaker(peerID, true)
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
		audio := int16ToBytes(mono)
		durationMs := packetDurationMs(len(mono), sampleRate)
		now := time.Now()
		frameEnergy := measureFrameEnergy(mono)

		if !s.canProcessAudio(peerID) {
			continue
		}

		var shouldStart bool
		var shouldEnd bool
		var shouldSend bool
		var prebuffer []byte

		peerState.mu.Lock()
		if peerState.prebuffer == nil || peerState.inputSampleRate != sampleRate {
			peerState.inputSampleRate = sampleRate
			peerState.prebuffer = newPCMRingBuffer(prebufferBytes(sampleRate, 1, prebufferSeconds))
		}
		if !peerState.speechActive {
			peerState.backgroundEnergies = appendEnergySample(peerState.backgroundEnergies, now, frameEnergy)
		} else {
			peerState.backgroundEnergies = pruneEnergySamples(peerState.backgroundEnergies, now)
		}
		if shouldRefreshSpeechThreshold(peerState.speechThresholdUpdatedAt, now) {
			peerState.speechThreshold = computeAdaptiveSpeechThreshold(peerState.backgroundEnergies)
			peerState.speechThresholdUpdatedAt = now
		}
		currentThreshold := effectiveSpeechThreshold(peerState.speechThreshold)
		isSpeech := isSpeechFrame(frameEnergy, currentThreshold)

		wasActive := peerState.speechActive
		if !peerState.speechActive {
			if isSpeech {
				peerState.voicedMs += durationMs
			} else {
				peerState.voicedMs = 0
			}
			if peerState.voicedMs >= vadStartThreshold {
				peerState.speechActive = true
				peerState.voicedMs = 0
				peerState.silenceMs = 0
				shouldStart = true
				log.Printf("rtc: speech start sample_rate=%d energy=%d threshold=%d", sampleRate, frameEnergy, currentThreshold)
				prebuffer = peerState.prebuffer.snapshot()
			}
		} else {
			if isSpeech {
				peerState.silenceMs = 0
			} else {
				peerState.silenceMs += durationMs
			}
			if peerState.silenceMs >= vadEndThreshold {
				peerState.speechActive = false
				peerState.silenceMs = 0
				shouldEnd = true
				log.Printf("rtc: speech end")
			}
		}
		shouldSend = wasActive || peerState.speechActive
		peerState.prebuffer.append(audio)
		peerState.mu.Unlock()

		if shouldStart {
			if s.activateSpeaker(peerID) {
				s.mu.Lock()
				s.cancelSpeechStopLocked()
				s.mu.Unlock()
				s.emit(types.Event{Kind: types.EventSpeechStart, Payload: types.SpeechEvent{Source: "server-vad", CapturedAt: time.Now()}})
				s.startSpeechStream(int32(sampleRate), 1, prebuffer)
			} else {
				peerState.mu.Lock()
				peerState.speechActive = false
				peerState.voicedMs = 0
				peerState.silenceMs = 0
				peerState.mu.Unlock()
			}
		}
		if shouldSend && s.isActiveSpeaker(peerID) {
			s.sendSpeechAudio(audio)
		}
		if shouldEnd && s.isActiveSpeaker(peerID) {
			s.emit(types.Event{Kind: types.EventSpeechEnd, Payload: types.SpeechEvent{Source: "server-vad", CapturedAt: time.Now()}})
			s.scheduleSpeechStop()
			s.clearActiveSpeaker(peerID, false)
		}
	}
}

func (s *stage) startSpeechStream(sampleRate int32, channels int32, prebuffer []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cancelSpeechStopLocked()
	s.stopSpeechLocked()

	projectID := strings.TrimSpace(s.cfg.SpeechProjectID)
	if projectID == "" {
		log.Printf("rtc: GOOGLE_CLOUD_PROJECT is empty; server-side STT disabled")
		return
	}
	if s.speechClient == nil {
		opts := make([]option.ClientOption, 0, 2)
		opts = append(opts, option.WithEndpoint(fmt.Sprintf("%s-speech.googleapis.com:443", speechRegion)))
		if strings.TrimSpace(s.cfg.SpeechCredsJSON) != "" {
			opts = append(opts, option.WithCredentialsJSON([]byte(s.cfg.SpeechCredsJSON)))
		}
		client, err := speech.NewClient(s.ctx, opts...)
		if err != nil {
			log.Printf("rtc: speech client create error: %v", err)
			return
		}
		s.speechClient = client
	}

	speechCtx, cancel := context.WithCancel(s.ctx)
	stream, err := s.speechClient.StreamingRecognize(speechCtx)
	if err != nil {
		cancel()
		log.Printf("rtc: speech streaming create error: %v", err)
		return
	}
	req := &speechpb.StreamingRecognizeRequest{
		Recognizer: recognizerPath(projectID, s.cfg.SpeechRecognizer),
		StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
			StreamingConfig: &speechpb.StreamingRecognitionConfig{
				Config: &speechpb.RecognitionConfig{
					Model: speechModel,
					DecodingConfig: &speechpb.RecognitionConfig_ExplicitDecodingConfig{
						ExplicitDecodingConfig: &speechpb.ExplicitDecodingConfig{
							Encoding:          speechpb.ExplicitDecodingConfig_LINEAR16,
							SampleRateHertz:   sampleRate,
							AudioChannelCount: channels,
						},
					},
					LanguageCodes: []string{s.cfg.SpeechLanguage},
				},
			},
		},
	}
	if err := stream.Send(req); err != nil {
		_ = stream.CloseSend()
		cancel()
		log.Printf("rtc: speech stream config send error: %v", err)
		return
	}

	s.speechStream = stream
	s.speechCancel = cancel
	go s.consumeSpeechResponses(stream)

	if len(prebuffer) > 0 {
		go s.sendSpeechAudio(prebuffer)
	}
}

func (s *stage) consumeSpeechResponses(stream speechpb.Speech_StreamingRecognizeClient) {
	for {
		resp, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) {
				log.Printf("rtc: speech stream recv error: %v", err)
			}
			return
		}
		for _, result := range resp.Results {
			if !result.IsFinal {
				continue
			}
			if len(result.Alternatives) == 0 {
				continue
			}
			text := strings.TrimSpace(result.Alternatives[0].Transcript)
			if text == "" {
				continue
			}
			s.emit(types.Event{Kind: types.EventTextInput, Payload: types.OutputLine{Role: "user", Text: text, Source: "server-stt"}})
		}
	}
}

func (s *stage) sendSpeechAudio(audio []byte) {
	if len(audio) == 0 {
		return
	}
	s.mu.Lock()
	stream := s.speechStream
	s.mu.Unlock()
	if stream == nil {
		return
	}
	for start := 0; start < len(audio); start += speechAudioChunkBytes {
		end := min(start+speechAudioChunkBytes, len(audio))
		if (end-start)%2 != 0 {
			end--
		}
		if end <= start {
			continue
		}
		req := &speechpb.StreamingRecognizeRequest{
			StreamingRequest: &speechpb.StreamingRecognizeRequest_Audio{
				Audio: audio[start:end],
			},
		}
		if err := stream.Send(req); err != nil {
			log.Printf("rtc: speech audio send error: %v", err)
			s.mu.Lock()
			if s.speechStream == stream {
				s.stopSpeechLocked()
			}
			s.mu.Unlock()
			return
		}
	}
}

func (s *stage) scheduleSpeechStop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelSpeechStopLocked()
	s.speechTimer = time.AfterFunc(sttStopDelay, func() {
		s.mu.Lock()
		s.closeSpeechSendLocked()
		s.mu.Unlock()
	})
}

func (s *stage) cancelSpeechStopLocked() {
	if s.speechTimer == nil {
		return
	}
	s.speechTimer.Stop()
	s.speechTimer = nil
}

func (s *stage) stopSpeechLocked() {
	s.cancelSpeechStopLocked()
	s.closeSpeechSendLocked()
	if s.speechCancel != nil {
		s.speechCancel()
		s.speechCancel = nil
	}
}

func (s *stage) closeSpeechSendLocked() {
	s.cancelSpeechStopLocked()
	if s.speechStream != nil {
		_ = s.speechStream.CloseSend()
		s.speechStream = nil
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

type energySample struct {
	energy     int
	capturedAt time.Time
}

func measureFrameEnergy(pcm []int16) int {
	if len(pcm) == 0 {
		return 0
	}
	var sum int64
	for _, sample := range pcm {
		v := int64(sample)
		if v < 0 {
			v = -v
		}
		sum += v
	}
	return int(sum / int64(len(pcm)))
}

func appendEnergySample(samples []energySample, capturedAt time.Time, energy int) []energySample {
	if energy < 0 {
		energy = 0
	}
	samples = append(samples, energySample{energy: energy, capturedAt: capturedAt})
	return pruneEnergySamples(samples, capturedAt)
}

func pruneEnergySamples(samples []energySample, now time.Time) []energySample {
	if len(samples) == 0 {
		return nil
	}
	cutoff := now.Add(-adaptiveVADHistoryWindow)
	first := 0
	for first < len(samples) && samples[first].capturedAt.Before(cutoff) {
		first++
	}
	if first == 0 {
		return samples
	}
	if first >= len(samples) {
		return nil
	}
	pruned := make([]energySample, len(samples)-first)
	copy(pruned, samples[first:])
	return pruned
}

func computeAdaptiveSpeechThreshold(samples []energySample) int {
	if len(samples) == 0 {
		return adaptiveVADMinThreshold
	}
	energies := make([]int, len(samples))
	for i, sample := range samples {
		energies[i] = sample.energy
	}
	sort.Ints(energies)
	mid := len(energies) / 2
	median := energies[mid]
	if len(energies)%2 == 0 {
		median = (energies[mid-1] + energies[mid]) / 2
	}
	threshold := int(math.Round(float64(median) * adaptiveVADThresholdMultiplier))
	return effectiveSpeechThreshold(threshold)
}

func effectiveSpeechThreshold(threshold int) int {
	if threshold < adaptiveVADMinThreshold {
		return adaptiveVADMinThreshold
	}
	return threshold
}

func shouldRefreshSpeechThreshold(last, now time.Time) bool {
	if last.IsZero() {
		return true
	}
	return !last.Add(adaptiveVADThresholdRefreshInterval).After(now)
}

func isSpeechFrame(energy int, threshold int) bool {
	return energy >= effectiveSpeechThreshold(threshold)
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

func prebufferBytes(sampleRate int, channels int, seconds int) int {
	if sampleRate <= 0 {
		sampleRate = webrtcSampleRate
	}
	if channels <= 0 {
		channels = 1
	}
	if seconds <= 0 {
		seconds = prebufferSeconds
	}
	return sampleRate * channels * seconds * 2
}

type pcmRingBuffer struct {
	buf   []byte
	head  int
	size  int
	limit int
}

func newPCMRingBuffer(limit int) *pcmRingBuffer {
	if limit <= 0 {
		limit = prebufferBytes(webrtcSampleRate, 1, prebufferSeconds)
	}
	return &pcmRingBuffer{buf: make([]byte, limit), limit: limit}
}

func (r *pcmRingBuffer) append(data []byte) {
	if r == nil || len(data) == 0 {
		return
	}
	if len(data) >= r.limit {
		copy(r.buf, data[len(data)-r.limit:])
		r.head = 0
		r.size = r.limit
		return
	}
	remaining := r.limit - r.head
	if len(data) <= remaining {
		copy(r.buf[r.head:], data)
		r.head += len(data)
		if r.head == r.limit {
			r.head = 0
		}
	} else {
		copy(r.buf[r.head:], data[:remaining])
		copy(r.buf, data[remaining:])
		r.head = len(data) - remaining
	}
	r.size += len(data)
	if r.size > r.limit {
		r.size = r.limit
	}
}

func (r *pcmRingBuffer) snapshot() []byte {
	if r == nil || r.size == 0 {
		return nil
	}
	out := make([]byte, r.size)
	start := r.head - r.size
	if start < 0 {
		start += r.limit
	}
	if start+r.size <= r.limit {
		copy(out, r.buf[start:start+r.size])
		return out
	}
	firstLen := r.limit - start
	copy(out[:firstLen], r.buf[start:])
	copy(out[firstLen:], r.buf[:r.size-firstLen])
	return out
}

func recognizerPath(projectID, recognizer string) string {
	id := strings.TrimSpace(recognizer)
	if id == "" {
		id = "_"
	}
	return fmt.Sprintf("projects/%s/locations/%s/recognizers/%s", strings.TrimSpace(projectID), speechRegion, id)
}
