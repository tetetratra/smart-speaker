package rtc

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	speech "cloud.google.com/go/speech/apiv2"
	speechpb "cloud.google.com/go/speech/apiv2/speechpb"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"google.golang.org/api/option"
	opus "gopkg.in/hraban/opus.v2"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

const (
	webrtcSampleRate = 48000
	webrtcChannels   = 1
	opusFrameMs      = 20

	prebufferSeconds   = 3
	vadStartThreshold  = 200
	vadEndThreshold    = 800
	sttStopDelay       = 1500 * time.Millisecond
	energySpeechThresh = 200
	speechAudioChunkBytes = 25600
	speechModel        = "chirp_3"
	speechRegion       = "asia-northeast1"
)

type Config struct {
	IceHostIPs       []string
	SpeechProjectID  string
	SpeechRecognizer string
	SpeechLanguage   string
	SpeechCredsJSON  string
}

func NewStage(cfg Config) (*graph.Stage, error) {
	if strings.TrimSpace(cfg.SpeechRecognizer) == "" {
		cfg.SpeechRecognizer = "_"
	}
	if strings.TrimSpace(cfg.SpeechLanguage) == "" {
		cfg.SpeechLanguage = "ja-JP"
	}
	r := &stage{
		cfg:        cfg,
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	return &graph.Stage{
		Upstream:   r.upstream,
		Downstream: r.downstream,
		Run:        r.run,
		CloseFn:    r.close,
	}, nil
}

type stage struct {
	cfg Config

	upstream   chan types.Event
	downstream chan types.Event

	ctx    context.Context
	cancel context.CancelFunc

	mu           sync.Mutex
	peer         *webrtc.PeerConnection
	track        *webrtc.TrackLocalStaticSample
	encoder      *opus.Encoder
	pendingICE   []webrtc.ICECandidateInit
	audioBuf     []int16
	connected    bool
	closed       bool
	opusChannels int

	speechClient *speech.Client
	speechStream speechpb.Speech_StreamingRecognizeClient
	speechCancel context.CancelFunc
	speechTimer  *time.Timer

	inputSampleRate int
	prebuffer       *pcmRingBuffer
	speechActive    bool
	voicedMs        int
	silenceMs       int
}

func (s *stage) run(parent context.Context) {
	s.ctx, s.cancel = context.WithCancel(parent)
	go s.consume()
	go s.sendLoop()
}

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
	if s.peer == nil || s.track == nil || s.encoder == nil || !s.connected {
		s.mu.Unlock()
		return
	}
	frameSize := webrtcSampleRate * opusFrameMs / 1000 * max(1, s.opusChannels)
	if len(s.audioBuf) < frameSize {
		s.mu.Unlock()
		return
	}
	frame := make([]int16, frameSize)
	copy(frame, s.audioBuf[:frameSize])
	s.audioBuf = s.audioBuf[frameSize:]
	track := s.track
	encoder := s.encoder
	s.mu.Unlock()

	opusBuf := make([]byte, 4000)
	n, err := encoder.Encode(frame, opusBuf)
	if err != nil {
		return
	}
	_ = track.WriteSample(media.Sample{Data: opusBuf[:n], Duration: time.Millisecond * opusFrameMs})
}

func (s *stage) consume() {
	defer close(s.downstream)
	for {
		select {
		case <-s.ctx.Done():
			return
		case evt, ok := <-s.upstream:
			if !ok {
				return
			}
			switch evt.Kind {
			case types.EventRTCSignal:
				sig, ok := evt.Payload.(types.RTCSignal)
				if !ok {
					continue
				}
				s.handleSignal(sig)
			case types.EventRealtimeAudio:
				audio, ok := evt.Payload.(types.OutputAudio)
				if !ok {
					continue
				}
				s.handleTTSAudio(audio)
			case types.EventTTSCancel:
				s.handleTTSCancel()
			}
		}
	}
}

func (s *stage) handleSignal(sig types.RTCSignal) {
	switch sig.Type {
	case "webrtc.offer":
		s.handleOffer(sig)
	case "webrtc.answer":
		s.handleAnswer(sig)
	case "webrtc.ice":
		s.handleICE(sig)
	}
}

func (s *stage) handleOffer(sig types.RTCSignal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetPeerLocked()

	peer, err := newPeerConnection(s.cfg.IceHostIPs)
	if err != nil {
		log.Printf("rtc: peer create error: %v", err)
		return
	}

	opusChannels := parseOpusChannels(sig.SDP)
	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: webrtcSampleRate, Channels: uint16(opusChannels)},
		"audio",
		"smart-speaker",
	)
	if err != nil {
		log.Printf("rtc: track create error: %v", err)
		return
	}
	if _, err := peer.AddTrack(track); err != nil {
		log.Printf("rtc: add track error: %v", err)
		return
	}

	peer.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		init := c.ToJSON()
		s.emit(types.Event{Kind: types.EventRTCSignal, Payload: types.RTCSignal{
			Type: "webrtc.ice",
			Candidate: &types.RTCIceCandidate{
				Candidate:     init.Candidate,
				SDPMid:        init.SDPMid,
				SDPMLineIndex: init.SDPMLineIndex,
			},
		}})
	})
	peer.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("rtc: connection state=%s", state.String())
		if state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateFailed {
			s.mu.Lock()
			s.connected = false
			s.stopSpeechLocked()
			s.mu.Unlock()
		}
	})
	peer.OnTrack(func(trackRemote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Printf("rtc: incoming track kind=%s codec=%s/%d channels=%d", trackRemote.Kind().String(), trackRemote.Codec().MimeType, trackRemote.Codec().ClockRate, trackRemote.Codec().Channels)
		go s.handleIncomingTrack(trackRemote)
	})

	encoder, err := opus.NewEncoder(webrtcSampleRate, opusChannels, opus.AppVoIP)
	if err != nil {
		log.Printf("rtc: opus encoder error: %v", err)
	}
	log.Printf("rtc: using opus channels=%d", opusChannels)

	s.peer = peer
	s.track = track
	s.encoder = encoder
	s.audioBuf = nil
	s.connected = true
	s.opusChannels = opusChannels

	if err := peer.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sig.SDP,
	}); err != nil {
		log.Printf("rtc: set remote desc error: %v", err)
		return
	}
	for _, cand := range s.pendingICE {
		if err := peer.AddICECandidate(cand); err != nil {
			log.Printf("rtc: add pending ice error: %v", err)
		}
	}
	s.pendingICE = nil

	answer, err := peer.CreateAnswer(nil)
	if err != nil {
		log.Printf("rtc: create answer error: %v", err)
		return
	}
	if err := peer.SetLocalDescription(answer); err != nil {
		log.Printf("rtc: set local desc error: %v", err)
		return
	}
	s.emit(types.Event{Kind: types.EventRTCSignal, Payload: types.RTCSignal{
		Type: "webrtc.answer",
		SDP:  answer.SDP,
	}})
}

func newPeerConnection(iceHostIPs []string) (*webrtc.PeerConnection, error) {
	var m webrtc.MediaEngine
	if err := m.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}
	var i interceptor.Registry
	if err := webrtc.RegisterDefaultInterceptors(&m, &i); err != nil {
		return nil, err
	}
	var s webrtc.SettingEngine
	icePortMin := uint16(50000)
	icePortMax := uint16(50100)
	if err := s.SetEphemeralUDPPortRange(icePortMin, icePortMax); err != nil {
		return nil, err
	}
	log.Printf("rtc: use ICE UDP port range %d-%d", icePortMin, icePortMax)
	if len(iceHostIPs) > 0 {
		log.Printf("rtc: use ICE host IPs: %s", strings.Join(iceHostIPs, ","))
		s.SetNAT1To1IPs(iceHostIPs, webrtc.ICECandidateTypeHost)
	}
	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(&m),
		webrtc.WithInterceptorRegistry(&i),
		webrtc.WithSettingEngine(s),
	)
	return api.NewPeerConnection(webrtc.Configuration{})
}

func (s *stage) handleAnswer(sig types.RTCSignal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.peer == nil {
		return
	}
	if err := s.peer.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sig.SDP,
	}); err != nil {
		log.Printf("rtc: set remote answer error: %v", err)
	}
}

func (s *stage) handleICE(sig types.RTCSignal) {
	if sig.Candidate == nil {
		return
	}
	init := webrtc.ICECandidateInit{
		Candidate:     sig.Candidate.Candidate,
		SDPMid:        sig.Candidate.SDPMid,
		SDPMLineIndex: sig.Candidate.SDPMLineIndex,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.peer == nil {
		s.pendingICE = append(s.pendingICE, init)
		return
	}
	if err := s.peer.AddICECandidate(init); err != nil {
		log.Printf("rtc: add ice error: %v", err)
	}
}

func (s *stage) handleIncomingTrack(trackRemote *webrtc.TrackRemote) {
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

	s.mu.Lock()
	s.inputSampleRate = sampleRate
	s.prebuffer = newPCMRingBuffer(prebufferBytes(sampleRate, 1, prebufferSeconds))
	s.speechActive = false
	s.voicedMs = 0
	s.silenceMs = 0
	s.cancelSpeechStopLocked()
	s.stopSpeechLocked()
	s.mu.Unlock()

	for {
		pkt, _, err := trackRemote.ReadRTP()
		if err != nil {
			log.Printf("rtc: incoming audio read error: %v", err)
			s.mu.Lock()
			s.speechActive = false
			s.voicedMs = 0
			s.silenceMs = 0
			s.stopSpeechLocked()
			s.mu.Unlock()
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
		isSpeech := detectSpeech(mono)

		var shouldStart bool
		var shouldEnd bool
		var shouldSend bool
		var prebuffer []byte

		s.mu.Lock()
		if s.prebuffer == nil || s.inputSampleRate != sampleRate {
			s.inputSampleRate = sampleRate
			s.prebuffer = newPCMRingBuffer(prebufferBytes(sampleRate, 1, prebufferSeconds))
		}

		wasActive := s.speechActive
		if !s.speechActive {
			if isSpeech {
				s.voicedMs += durationMs
			} else {
				s.voicedMs = 0
			}
			if s.voicedMs >= vadStartThreshold {
				s.speechActive = true
				s.voicedMs = 0
				s.silenceMs = 0
				shouldStart = true
				log.Printf("rtc: speech start sample_rate=%d avg_energy_trigger=%d", sampleRate, energySpeechThresh)
				prebuffer = s.prebuffer.snapshot()
				s.cancelSpeechStopLocked()
			}
		} else {
			if isSpeech {
				s.silenceMs = 0
			} else {
				s.silenceMs += durationMs
			}
			if s.silenceMs >= vadEndThreshold {
				s.speechActive = false
				s.silenceMs = 0
				shouldEnd = true
				log.Printf("rtc: speech end")
			}
		}
		shouldSend = wasActive || s.speechActive
		s.prebuffer.append(audio)
		s.mu.Unlock()

		if shouldStart {
			s.emit(types.Event{Kind: types.EventSpeechStart, Payload: types.SpeechEvent{Source: "server-vad", CapturedAt: time.Now()}})
			s.startSpeechStream(int32(sampleRate), 1, prebuffer)
		}
		if shouldSend {
			s.sendSpeechAudio(audio)
		}
		if shouldEnd {
			s.emit(types.Event{Kind: types.EventSpeechEnd, Payload: types.SpeechEvent{Source: "server-vad", CapturedAt: time.Now()}})
			s.scheduleSpeechStop()
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

func (s *stage) handleTTSAudio(audio types.OutputAudio) {
	s.mu.Lock()
	if s.peer == nil || s.track == nil || s.encoder == nil || !s.connected {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

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
	if s.opusChannels == 2 {
		pcm = upmixToStereo(pcm)
	}

	s.mu.Lock()
	s.audioBuf = append(s.audioBuf, pcm...)
	s.mu.Unlock()
}

func (s *stage) handleTTSCancel() {
	s.mu.Lock()
	s.audioBuf = nil
	s.mu.Unlock()
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

func parseOpusChannels(sdp string) int {
	lines := strings.Split(sdp, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "a=rtpmap:") && strings.Contains(line, "opus/48000/") {
			parts := strings.Split(line, "opus/48000/")
			if len(parts) < 2 {
				break
			}
			ch := strings.TrimSpace(parts[1])
			if ch == "1" {
				return 1
			}
			if ch == "2" {
				return 2
			}
		}
	}
	return webrtcChannels
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

func detectSpeech(pcm []int16) bool {
	if len(pcm) == 0 {
		return false
	}
	var sum int64
	for _, sample := range pcm {
		v := int64(sample)
		if v < 0 {
			v = -v
		}
		sum += v
	}
	avg := sum / int64(len(pcm))
	return avg >= energySpeechThresh
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *stage) resetPeerLocked() {
	if s.peer != nil {
		_ = s.peer.Close()
		s.peer = nil
	}
	s.stopSpeechLocked()
	s.encoder = nil
	s.track = nil
	s.pendingICE = nil
	s.audioBuf = nil
	s.prebuffer = nil
	s.speechActive = false
	s.voicedMs = 0
	s.silenceMs = 0
	s.connected = false
}

func (s *stage) emit(evt types.Event) {
	select {
	case <-s.ctx.Done():
		return
	case s.downstream <- evt:
	}
}

func (s *stage) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	s.stopSpeechLocked()
	if s.speechClient != nil {
		if err := s.speechClient.Close(); err != nil {
			log.Printf("rtc: speech client close error: %v", err)
		}
		s.speechClient = nil
	}
	s.resetPeerLocked()
	close(s.upstream)
	return nil
}
