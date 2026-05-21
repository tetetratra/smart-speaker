package conversation

import (
	"encoding/json"
	"strconv"

	types "smart-speaker/internal/types"
)

type sessionState struct {
	conversation          []*Utterance
	current               *Utterance
	utteranceByResponseID map[string]*Utterance

	pendingTimeline    []timelineSegment
	pendingTimelineIdx int

	pendingRequestID       string
	invalidResponseRetries int
	requestGeneration      map[string]uint64

	pendingRequestStreaming     bool
	pendingStreamSpeechStarted  bool
	pendingStreamFailed         bool
	pendingStreamToolSeen       bool
	pendingTimelineTimerWaiting bool
	pendingStreamLines          []string

	seq        int
	generation uint64
}

type timelineSegment struct {
	Type    string
	WaitSec int
	Text    string
	Tool    *toolCallSegment
}

type toolCallSegment struct {
	Name string
	Args json.RawMessage
}

func newSessionState() *sessionState {
	return &sessionState{
		utteranceByResponseID: make(map[string]*Utterance),
		requestGeneration:     make(map[string]uint64),
	}
}

func (s *sessionState) nextID(prefix string) string {
	s.seq++
	return prefix + "_" + strconv.Itoa(s.seq)
}

func (s *sessionState) nextGeneration() uint64 {
	s.generation++
	return s.generation
}

func (s *sessionState) currentGeneration() uint64 {
	return s.generation
}

func (s *sessionState) buildConversationMessages() []types.ChatMessage {
	var out []types.ChatMessage
	for _, utt := range s.conversation {
		if utt == nil {
			continue
		}
		switch utt.Speaker {
		case SpeakerHuman:
			out = append(out, types.ChatMessage{Role: "user", Content: utt.Content})
		case SpeakerAI:
			// assistant 発話は EventRealtimeOutput を出した時点で利用者に提示済みなので、
			// TTS 完了前の追い質問でも文脈に残す。
			if utt.Status == UtteranceUnplayed {
				continue
			}
			out = append(out, types.ChatMessage{Role: "assistant", Content: utt.Content})
		case SpeakerTool:
			out = append(out, types.ChatMessage{Role: "system", Content: utt.Content})
		}
	}
	return out
}

func (s *sessionState) appendUtterance(utt *Utterance) {
	if utt == nil {
		return
	}
	s.conversation = append(s.conversation, utt)
}

func (s *sessionState) cancelPendingRequest() {
	if s.pendingRequestID == "" {
		return
	}
	delete(s.requestGeneration, s.pendingRequestID)
	s.pendingRequestID = ""
	s.pendingRequestStreaming = false
	s.pendingStreamSpeechStarted = false
	s.pendingStreamFailed = false
	s.pendingStreamToolSeen = false
	s.clearPendingStreamLines()
}

func (s *sessionState) clearPendingTimeline() {
	s.pendingTimeline = nil
	s.pendingTimelineIdx = 0
	s.pendingTimelineTimerWaiting = false
}

func (s *sessionState) clearPendingStreamLines() {
	s.pendingStreamLines = nil
}

func (s *sessionState) hasPendingSpeech() bool {
	for i := s.pendingTimelineIdx; i < len(s.pendingTimeline); i++ {
		if s.pendingTimeline[i].Type == "speech" {
			return true
		}
	}
	return false
}

func (s *sessionState) hasPendingTimelineWork() bool {
	for i := s.pendingTimelineIdx; i < len(s.pendingTimeline); i++ {
		switch s.pendingTimeline[i].Type {
		case "speech", "tool":
			return true
		}
	}
	return false
}

func (s *sessionState) consumeLeadingWaitSeconds() float64 {
	var total float64
	for s.pendingTimelineIdx < len(s.pendingTimeline) {
		seg := s.pendingTimeline[s.pendingTimelineIdx]
		if seg.Type != "wait" {
			break
		}
		total += float64(seg.WaitSec)
		s.pendingTimelineIdx++
	}
	return total
}

func (s *sessionState) resetConversation() {
	s.current = nil
	s.utteranceByResponseID = make(map[string]*Utterance)
	s.conversation = nil
	s.clearPendingTimeline()
	s.pendingRequestID = ""
	s.invalidResponseRetries = 0
	s.requestGeneration = make(map[string]uint64)
	s.pendingRequestStreaming = false
	s.pendingStreamSpeechStarted = false
	s.pendingStreamFailed = false
	s.pendingStreamToolSeen = false
	s.clearPendingStreamLines()
}
