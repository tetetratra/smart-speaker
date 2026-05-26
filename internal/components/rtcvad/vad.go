package rtcvad

import (
	"log"
	"sort"
	"time"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func (s *stage) handleAudioFrame(frame types.RTCPeerAudioFrame) {
	peerID := normalizePeerID(frame.PeerID)
	if len(frame.Samples) == 0 || len(frame.PCM) == 0 {
		return
	}
	sampleRate := frame.SampleRate
	if sampleRate <= 0 {
		sampleRate = webrtcSampleRate
	}
	durationMs := frame.DurationMs
	if durationMs <= 0 {
		durationMs = packetDurationMs(len(frame.Samples), sampleRate)
	}
	now := frame.CapturedAt
	if now.IsZero() {
		now = time.Now()
	}
	frameEnergy := measureFrameEnergy(frame.Samples)

	if !s.canProcessAudio(peerID) {
		return
	}

	peerState := s.getOrCreatePeer(peerID)
	var shouldStart bool
	var shouldEnd bool
	var shouldSend bool
	var prebuffer []byte

	peerState.mu.Lock()
	if peerState.prebuffer == nil || peerState.inputSampleRate != sampleRate {
		peerState.inputSampleRate = sampleRate
		peerState.prebuffer = newPCMRingBuffer(prebufferBytes(sampleRate, 1, prebufferSeconds))
	}
	peerState.backgroundEnergies = appendEnergySample(peerState.backgroundEnergies, now, frameEnergy)
	if shouldRefreshSpeechThreshold(peerState.speechThresholdUpdatedAt, now) {
		peerState.speechThreshold = computeAdaptiveSpeechThreshold(peerState.backgroundEnergies)
		peerState.speechThresholdUpdatedAt = now
	}
	currentThreshold := effectiveSpeechThreshold(peerState.speechThreshold)
	isSpeech := isSpeechFrame(frameEnergy, currentThreshold)
	shouldEmitStatus := shouldEmitVADStatus(peerState.lastVADStatusSentAt, now)
	if shouldEmitStatus {
		peerState.lastVADStatusSentAt = now
	}

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
			log.Printf("rtcvad: speech start sample_rate=%d energy=%d threshold=%d", sampleRate, frameEnergy, currentThreshold)
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
			log.Printf("rtcvad: speech end")
		}
	}
	shouldSend = wasActive || peerState.speechActive
	peerState.prebuffer.append(frame.PCM)
	peerState.mu.Unlock()

	if shouldEmitStatus {
		s.emit(types.Event{
			Kind: types.EventRTCVADStatus,
			Payload: types.RTCVADStatus{
				InputLevel: frameEnergy,
				Threshold:  currentThreshold,
				CapturedAt: now,
			},
		})
	}

	if shouldStart {
		if s.activateSpeaker(peerID) {
			s.emit(types.Event{Kind: types.EventRTCSpeechAudio, Payload: types.RTCSpeechAudio{
				PeerID:     peerID,
				Type:       types.RTCSpeechAudioStart,
				Prebuffer:  prebuffer,
				SampleRate: sampleRate,
				Channels:   1,
				CapturedAt: now,
			}})
		} else {
			peerState.mu.Lock()
			peerState.speechActive = false
			peerState.voicedMs = 0
			peerState.silenceMs = 0
			peerState.mu.Unlock()
		}
	}
	if shouldSend && s.isActiveSpeaker(peerID) {
		s.emit(types.Event{Kind: types.EventRTCSpeechAudio, Payload: types.RTCSpeechAudio{
			PeerID:     peerID,
			Type:       types.RTCSpeechAudioFrame,
			PCM:        frame.PCM,
			SampleRate: sampleRate,
			Channels:   1,
			CapturedAt: now,
		}})
	}
	if shouldEnd && s.isActiveSpeaker(peerID) {
		s.emit(types.Event{Kind: types.EventSpeechEnd, Payload: types.SpeechEvent{Source: "server-vad", CapturedAt: time.Now()}})
		s.emit(types.Event{Kind: types.EventRTCSpeechAudio, Payload: types.RTCSpeechAudio{
			PeerID:     peerID,
			Type:       types.RTCSpeechAudioEnd,
			SampleRate: sampleRate,
			Channels:   1,
			CapturedAt: time.Now(),
		}})
		s.clearActiveSpeaker(peerID)
	}
}

func (s *stage) getOrCreatePeer(id string) *peerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.peers == nil {
		s.peers = map[string]*peerState{}
	}
	if peer, ok := s.peers[id]; ok && peer != nil {
		return peer
	}
	peer := &peerState{speechThreshold: adaptiveVADMinThreshold}
	s.peers[id] = peer
	return peer
}

func (s *stage) canProcessAudio(peerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeSpeakerID == "" || s.activeSpeakerID == peerID
}

func (s *stage) isActiveSpeaker(peerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeSpeakerID == peerID
}

func (s *stage) activateSpeaker(peerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeSpeakerID == "" || s.activeSpeakerID == peerID {
		s.activeSpeakerID = peerID
		return true
	}
	return false
}

func (s *stage) clearActiveSpeaker(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeSpeakerID == peerID {
		s.activeSpeakerID = ""
	}
}

func normalizePeerID(peerID string) string {
	if peerID == "" {
		return "default"
	}
	return peerID
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
	threshold := median + adaptiveVADThresholdOffset
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

func shouldEmitVADStatus(last, now time.Time) bool {
	if last.IsZero() {
		return true
	}
	return !last.Add(adaptiveVADStatusEmitInterval).After(now)
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
