package conversation

import (
	"strconv"

	types "smart-speaker/internal/types"
)

type sessionState struct {
	conversation          []*Utterance
	current               *Utterance
	utteranceByResponseID map[string]*Utterance

	pendingTimeline    []timelineSegment
	pendingTimelineIdx int

	pendingRequestID        string
	pendingRequestCancelled bool
	invalidResponseRetries  int

	pendingRequestStreaming     bool
	pendingStreamSpeechStarted  bool
	pendingStreamFailed         bool
	pendingTimelineTimerWaiting bool

	seq int
}

type timelineSegment struct {
	Type    string
	WaitSec int
	Text    string
}

func newSessionState() *sessionState {
	return &sessionState{
		utteranceByResponseID: make(map[string]*Utterance),
	}
}

func (s *sessionState) nextID(prefix string) string {
	s.seq++
	return prefix + "_" + strconv.Itoa(s.seq)
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
			if utt.Status != UtterancePlayed {
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
	s.pendingRequestCancelled = true
}

func (s *sessionState) cancelUnplayedUtterances() {
	for _, utt := range s.conversation {
		if utt == nil || utt.Speaker != SpeakerAI {
			continue
		}
		if utt.Status == UtterancePlayed {
			continue
		}
		utt.Status = UtteranceCanceled
	}
}

func (s *sessionState) clearPendingTimeline() {
	s.pendingTimeline = nil
	s.pendingTimelineIdx = 0
	s.pendingTimelineTimerWaiting = false
}

func (s *sessionState) hasPendingSpeech() bool {
	for i := s.pendingTimelineIdx; i < len(s.pendingTimeline); i++ {
		if s.pendingTimeline[i].Type == "speech" {
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
	s.pendingRequestCancelled = false
	s.invalidResponseRetries = 0
	s.pendingRequestStreaming = false
	s.pendingStreamSpeechStarted = false
	s.pendingStreamFailed = false
}
