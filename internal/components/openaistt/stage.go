package openaistt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	"github.com/tetetratra/smart-speaker/internal/graph"
	types "github.com/tetetratra/smart-speaker/internal/types"
	"nhooyr.io/websocket"
)

const defaultModel = "gpt-realtime-whisper"

type Config struct {
	APIKey   string
	Model    string
	Phrases  []string
	Endpoint string

	dialer realtimeDialer
}

func NewStage(cfg Config) (*graph.Stage, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("openaistt: OPENAI_API_KEY is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = defaultModel
	}
	if cfg.dialer == nil {
		cfg.dialer = websocketDialer{}
	}
	s := &stage{
		cfg:        cfg,
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	return &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.close,
	}, nil
}

type stage struct {
	cfg Config

	upstream   chan types.Event
	downstream chan types.Event

	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	closed  bool
	session *speechSession
}

type speechSession struct {
	ctx    context.Context
	cancel context.CancelFunc
	conn   realtimeConn

	sampleRate int
	channels   int
	committed  bool
}

type realtimeEvent struct {
	Type       string          `json:"type"`
	ItemID     string          `json:"item_id"`
	Delta      string          `json:"delta"`
	Transcript string          `json:"transcript"`
	Error      json.RawMessage `json:"error"`
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
			if evt.Kind != types.EventRTCSpeechAudio {
				continue
			}
			audio, ok := evt.Payload.(types.RTCSpeechAudio)
			if !ok {
				continue
			}
			s.handleSpeechAudio(audio)
		}
	}
}

func (s *stage) handleSpeechAudio(audio types.RTCSpeechAudio) {
	switch audio.Type {
	case types.RTCSpeechAudioStart:
		s.startSpeechSession(audio.SampleRate, audio.Channels, audio.Prebuffer)
	case types.RTCSpeechAudioFrame:
		s.sendSpeechAudio(audio.PCM)
	case types.RTCSpeechAudioEnd:
		s.commitSpeechAudio()
	}
}

func (s *stage) startSpeechSession(sampleRate int, channels int, prebuffer []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stopSessionLocked()
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if channels <= 0 {
		channels = 1
	}

	sessionCtx, cancel := context.WithCancel(s.ctx)
	conn, err := s.cfg.dialer.Dial(sessionCtx, s.cfg)
	if err != nil {
		cancel()
		log.Printf("openaistt: websocket dial error: %v", err)
		return
	}
	if err := sendSessionUpdate(sessionCtx, conn, s.cfg); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "session update failed")
		cancel()
		log.Printf("openaistt: session update send error: %v", err)
		return
	}

	session := &speechSession{
		ctx:        sessionCtx,
		cancel:     cancel,
		conn:       conn,
		sampleRate: sampleRate,
		channels:   channels,
	}
	s.session = session
	go s.consumeRealtimeEvents(session)

	if len(prebuffer) > 0 {
		normalized := normalizePCM16(prebuffer, session.sampleRate, session.channels)
		if err := sendAudioAppend(session.ctx, session.conn, normalized); err != nil {
			log.Printf("openaistt: prebuffer append send error: %v", err)
			s.stopSessionLocked()
		}
	}
}

func (s *stage) sendSpeechAudio(pcm []byte) {
	if len(pcm) == 0 {
		return
	}
	s.mu.Lock()
	session := s.session
	s.mu.Unlock()
	if session == nil {
		return
	}
	normalized := normalizePCM16(pcm, session.sampleRate, session.channels)
	if len(normalized) == 0 {
		return
	}
	if err := sendAudioAppend(session.ctx, session.conn, normalized); err != nil {
		log.Printf("openaistt: audio append send error: %v", err)
		s.mu.Lock()
		if s.session == session {
			s.stopSessionLocked()
		}
		s.mu.Unlock()
	}
}

func (s *stage) commitSpeechAudio() {
	s.mu.Lock()
	session := s.session
	if session != nil && !session.committed {
		session.committed = true
	}
	s.mu.Unlock()
	if session == nil {
		return
	}
	if err := sendAudioCommit(session.ctx, session.conn); err != nil {
		log.Printf("openaistt: audio commit send error: %v", err)
		s.mu.Lock()
		if s.session == session {
			s.stopSessionLocked()
		}
		s.mu.Unlock()
	}
}

func (s *stage) consumeRealtimeEvents(session *speechSession) {
	transcripts := map[string]string{}
	for {
		_, data, err := session.conn.Read(session.ctx)
		if err != nil {
			if !isExpectedRealtimeClose(err) {
				log.Printf("openaistt: websocket read error: %v", err)
			}
			return
		}
		var evt realtimeEvent
		if err := json.Unmarshal(data, &evt); err != nil {
			log.Printf("openaistt: websocket event decode error: %v", err)
			continue
		}
		switch evt.Type {
		case "conversation.item.input_audio_transcription.delta":
			text := strings.TrimSpace(transcripts[evt.ItemID] + evt.Delta)
			transcripts[evt.ItemID] = text
			s.emitTranscript(types.EventHumanInterimUtterance, text, false)
		case "conversation.item.input_audio_transcription.completed":
			text := strings.TrimSpace(evt.Transcript)
			s.emitTranscript(types.EventHumanUtterance, text, true)
			s.mu.Lock()
			if s.session == session {
				s.stopSessionLocked()
			}
			s.mu.Unlock()
			return
		case "conversation.item.input_audio_transcription.failed":
			log.Printf("openaistt: transcription failed: %s", sanitizeEventError(evt.Error))
		case "error":
			log.Printf("openaistt: realtime error: %s", sanitizeEventError(evt.Error))
		}
	}
}

func (s *stage) emitTranscript(kind types.EventKind, text string, final bool) {
	if strings.TrimSpace(text) == "" {
		return
	}
	s.emit(types.Event{
		Kind: kind,
		Payload: types.OutputLine{
			Role:   "user",
			Text:   text,
			Final:  final,
			Source: "server-stt",
		},
	})
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
	s.stopSessionLocked()
	close(s.upstream)
	return nil
}

func (s *stage) stopSessionLocked() {
	if s.session == nil {
		return
	}
	_ = s.session.conn.Close(websocket.StatusNormalClosure, "bye")
	s.session.cancel()
	s.session = nil
}

func isExpectedRealtimeClose(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
		return true
	}
	status := websocket.CloseStatus(err)
	return status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway
}

func sanitizeEventError(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "unknown"
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "unparseable"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "unparseable"
	}
	return string(data)
}
