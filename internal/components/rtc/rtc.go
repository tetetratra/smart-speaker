package rtc

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	vosk "github.com/alphacep/vosk-api/go"
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	opus "gopkg.in/hraban/opus.v2"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

const (
	webrtcSampleRate  = 48000
	webrtcChannels    = 1
	sttSampleRate     = 16000
	opusFrameMs       = 20
	vadStartFrames    = 25   // 連続で音声と判定したフレーム数で開始扱い
	vadEndFrames      = 50   // 連続で無音と判定したフレーム数で終了扱い
	vadPreRollMs      = 200  // 発話開始直前に付与する音声の長さ
	vadMinSpeechRMS   = 300  // 音声と判定する最小RMS
	vadSpeechRatio    = 2.5  // ノイズ床に対する音声判定倍率
	vadNoiseMaxRatio  = 1.5  // ノイズ床更新を許す最大倍率
	vadNoiseSmoothing = 0.95 // ノイズ床の指数移動平均係数
)

type Config struct {
	ModelPath  string
	IceHostIPs []string
}

func NewStage(cfg Config) (*graph.Stage, error) {
	if strings.TrimSpace(cfg.ModelPath) == "" {
		return nil, errors.New("rtc: VOSK_MODEL_PATH is required")
	}
	model, err := vosk.NewModel(cfg.ModelPath)
	if err != nil {
		return nil, err
	}
	r := &stage{
		cfg:        cfg,
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		model:      model,
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
	decoder      *opus.Decoder
	recognizer   *vosk.VoskRecognizer
	pendingICE   []webrtc.ICECandidateInit
	audioBuf     []int16
	model        *vosk.VoskModel
	trackCancel  context.CancelFunc
	connected    bool
	closed       bool
	opusChannels int
}

func (s *stage) run(parent context.Context) {
	s.ctx, s.cancel = context.WithCancel(parent)
	go s.consume()
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
		if state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateFailed {
			s.mu.Lock()
			s.connected = false
			s.mu.Unlock()
		}
	})

	decoder, err := opus.NewDecoder(sttSampleRate, opusChannels)
	if err != nil {
		log.Printf("rtc: opus decoder error: %v", err)
	}
	encoder, err := opus.NewEncoder(webrtcSampleRate, opusChannels, opus.AppVoIP)
	if err != nil {
		log.Printf("rtc: opus encoder error: %v", err)
	}
	log.Printf("rtc: using opus channels=%d stt_rate=%d", opusChannels, sttSampleRate)

	recognizer, err := vosk.NewRecognizer(s.model, sttSampleRate)
	if err != nil {
		log.Printf("rtc: vosk recognizer error: %v", err)
	}

	trackCtx, trackCancel := context.WithCancel(s.ctx)
	s.trackCancel = trackCancel
	peer.OnTrack(func(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		go s.consumeRemoteTrack(trackCtx, remote, decoder, recognizer)
	})

	s.peer = peer
	s.track = track
	s.decoder = decoder
	s.encoder = encoder
	s.recognizer = recognizer
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

func (s *stage) consumeRemoteTrack(ctx context.Context, track *webrtc.TrackRemote, decoder *opus.Decoder, recognizer *vosk.VoskRecognizer) {
	if track == nil {
		return
	}
	if decoder == nil || recognizer == nil {
		return
	}
	vad := newVADState()
	preRollMax := sttSampleRate * vadPreRollMs / 1000
	preRoll := make([]int16, 0, preRollMax)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		pkt, _, err := track.ReadRTP()
		if err != nil {
			return
		}
		pcm, err := decodeOpusPacket(decoder, pkt, s.opusChannels, sttSampleRate)
		if err != nil {
			continue
		}
		if len(pcm) == 0 {
			continue
		}
		sttPCM := downmixToMono(pcm, s.opusChannels)
		if len(sttPCM) == 0 {
			continue
		}
		active, started, ended := vad.update(rms(sttPCM))
		if started {
			combined := append(preRoll, sttPCM...)
			preRoll = preRoll[:0]
			s.feedRecognizer(recognizer, combined)
		} else if active {
			s.feedRecognizer(recognizer, sttPCM)
		} else {
			preRoll = append(preRoll, sttPCM...)
			if len(preRoll) > preRollMax {
				preRoll = preRoll[len(preRoll)-preRollMax:]
			}
		}
		if ended {
			preRoll = preRoll[:0]
			s.flushRecognizer(recognizer)
		}
	}
}

func (s *stage) handleTTSAudio(audio types.OutputAudio) {
	s.mu.Lock()
	if s.peer == nil || s.track == nil || s.encoder == nil || !s.connected {
		s.mu.Unlock()
		return
	}
	track := s.track
	encoder := s.encoder
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

	frameSize := webrtcSampleRate * opusFrameMs / 1000 * max(1, s.opusChannels)
	opusBuf := make([]byte, 4000)

	s.mu.Lock()
	s.audioBuf = append(s.audioBuf, pcm...)
	buf := s.audioBuf
	for len(buf) >= frameSize {
		frame := buf[:frameSize]
		buf = buf[frameSize:]
		n, err := encoder.Encode(frame, opusBuf)
		if err != nil {
			continue
		}
		s.mu.Unlock()
		_ = track.WriteSample(media.Sample{Data: opusBuf[:n], Duration: time.Millisecond * opusFrameMs})
		s.mu.Lock()
	}
	s.audioBuf = buf
	s.mu.Unlock()
}

func decodeOpusPacket(dec *opus.Decoder, pkt *rtp.Packet, channels int, sampleRate int) ([]int16, error) {
	if pkt == nil || dec == nil {
		return nil, errors.New("decoder not ready")
	}
	maxSamples := sampleRate * 60 / 1000 * max(1, channels)
	pcm := make([]int16, maxSamples)
	n, err := dec.Decode(pkt.Payload, pcm)
	if err != nil {
		return nil, err
	}
	return pcm[:n*max(1, channels)], nil
}

func extractVoskText(result string) string {
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Text)
}

func (s *stage) feedRecognizer(recognizer *vosk.VoskRecognizer, pcm []int16) {
	if recognizer == nil || len(pcm) == 0 {
		return
	}
	buf := int16ToBytes(pcm)
	if recognizer.AcceptWaveform(buf) != 0 {
		text := extractVoskText(recognizer.Result())
		if strings.TrimSpace(text) != "" {
			s.emit(types.Event{Kind: types.EventTextInput, Payload: types.OutputLine{
				Role:   "user",
				Text:   text,
				Source: "stt",
			}})
		}
	}
}

func (s *stage) flushRecognizer(recognizer *vosk.VoskRecognizer) {
	if recognizer == nil {
		return
	}
	text := extractVoskText(recognizer.FinalResult())
	if strings.TrimSpace(text) != "" {
		s.emit(types.Event{Kind: types.EventTextInput, Payload: types.OutputLine{
			Role:   "user",
			Text:   text,
			Source: "stt",
		}})
	}
}

func int16ToBytes(samples []int16) []byte {
	buf := make([]byte, len(samples)*2)
	for i, v := range samples {
		binary.LittleEndian.PutUint16(buf[i*2:], uint16(v))
	}
	return buf
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
	if channels <= 1 || len(in) == 0 {
		return in
	}
	out := make([]int16, len(in)/channels)
	for i := 0; i < len(out); i++ {
		left := int(in[i*channels])
		right := int(in[i*channels+1])
		out[i] = int16((left + right) / 2)
	}
	return out
}

type vadState struct {
	active       bool
	speechCount  int
	silenceCount int
	noiseRMS     float64
}

func newVADState() *vadState {
	return &vadState{}
}

func (v *vadState) update(rms float64) (active bool, started bool, ended bool) {
	if v.noiseRMS == 0 {
		v.noiseRMS = rms
	}
	if !v.active && rms <= v.noiseRMS*vadNoiseMaxRatio {
		v.noiseRMS = v.noiseRMS*vadNoiseSmoothing + rms*(1.0-vadNoiseSmoothing)
	}
	threshold := maxFloat(float64(vadMinSpeechRMS), v.noiseRMS*vadSpeechRatio)
	isSpeech := rms >= threshold
	if isSpeech {
		v.speechCount++
		v.silenceCount = 0
	} else {
		v.silenceCount++
		if !v.active {
			v.speechCount = 0
		}
	}
	if !v.active && v.speechCount >= vadStartFrames {
		v.active = true
		started = true
		v.speechCount = 0
		v.silenceCount = 0
	}
	if v.active && v.silenceCount >= vadEndFrames {
		v.active = false
		ended = true
		v.speechCount = 0
		v.silenceCount = 0
	}
	return v.active, started, ended
}

func rms(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sumSquares int64
	for _, sample := range samples {
		v := int64(sample)
		sumSquares += v * v
	}
	meanSquare := float64(sumSquares) / float64(len(samples))
	return math.Sqrt(meanSquare)
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
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

func (s *stage) resetPeerLocked() {
	if s.peer != nil {
		_ = s.peer.Close()
		s.peer = nil
	}
	if s.recognizer != nil {
		s.recognizer.Free()
		s.recognizer = nil
	}
	if s.trackCancel != nil {
		s.trackCancel()
		s.trackCancel = nil
	}
	s.encoder = nil
	s.decoder = nil
	s.track = nil
	s.pendingICE = nil
	s.audioBuf = nil
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
	s.resetPeerLocked()
	if s.model != nil {
		s.model.Free()
		s.model = nil
	}
	close(s.upstream)
	return nil
}
