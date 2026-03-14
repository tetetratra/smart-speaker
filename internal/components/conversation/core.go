package conversation

import (
	"strings"
	"time"

	types "smart-speaker/internal/types"
)

type conversationCore struct {
	state *sessionState
	rules []Rule
}

func newConversationCore() *conversationCore {
	core := &conversationCore{
		state: newSessionState(),
	}
	core.rules = defaultConversationRules()
	return core
}

type Rule interface {
	Apply(core *conversationCore, sig signal) ([]effect, bool)
}

func (c *conversationCore) Handle(sig signal) []effect {
	if sig == nil {
		return nil
	}
	for _, rule := range c.rules {
		effects, handled := rule.Apply(c, sig)
		if handled {
			return effects
		}
	}
	return nil
}

func (c *conversationCore) interruptCurrentConversationEffects() []effect {
	effects := []effect{stopTimerEffect{}}
	c.state.clearPendingTimeline()
	c.state.cancelPendingRequest()
	c.state.invalidResponseRetries = 0
	if c.state.current != nil && c.state.current.Status == UtterancePlaying {
		c.state.current.Status = UtteranceCanceled
		effects = append(effects, emitEventEffect{
			event: types.Event{
				Kind:    types.EventTTSCancel,
				Payload: types.TTSCancel{ResponseID: c.state.current.ResponseID},
			},
		})
		delete(c.state.utteranceByResponseID, c.state.current.ResponseID)
		c.state.current = nil
	}
	c.state.cancelUnplayedUtterances()
	return effects
}

func (c *conversationCore) buildResponseRequestEffect(messages []types.ChatMessage, tools []any) []effect {
	if len(messages) == 0 {
		return nil
	}
	reqID := c.state.nextID("req")
	c.state.pendingRequestID = reqID
	c.state.pendingRequestCancelled = false
	return []effect{requestResponseEffect{
		requestID: reqID,
		messages:  messages,
		tools:     tools,
	}}
}

func (c *conversationCore) retryInvalidResponseEffects() []effect {
	if c.state.invalidResponseRetries >= maxInvalidResponseRetries {
		return []effect{runtimeLogEffect{
			message: "conversation: invalid response retry exhausted (1/1)",
		}}
	}
	messages := c.state.buildConversationMessages()
	if len(messages) == 0 {
		return nil
	}
	messages = append(messages, types.ChatMessage{
		Role:    "system",
		Content: invalidResponseRetryHint,
	})
	c.state.invalidResponseRetries++
	effects := c.buildResponseRequestEffect(messages, []any{})
	effects = append(effects, runtimeLogEffect{
		message: "conversation: retrying due to invalid response (1/1)",
	})
	return effects
}

func (c *conversationCore) advanceTimelineEffects() []effect {
	for c.state.pendingTimelineIdx < len(c.state.pendingTimeline) {
		seg := c.state.pendingTimeline[c.state.pendingTimelineIdx]
		c.state.pendingTimelineIdx++
		switch seg.Type {
		case "wait":
			delay := waitDelay(seg.Sec)
			if delay > 0 {
				return []effect{startTimerEffect{duration: delay}}
			}
		case "speech":
			speech := sanitizeSpeech(seg.Text)
			if speech == "" {
				continue
			}
			utt := &Utterance{
				ID:      c.state.nextID("ai"),
				Speaker: SpeakerAI,
				Content: speech,
				Status:  UtteranceUnplayed,
			}
			c.state.appendUtterance(utt)
			return c.playUtteranceEffects(utt)
		}
	}
	c.state.clearPendingTimeline()
	return nil
}

func (c *conversationCore) playUtteranceEffects(utt *Utterance) []effect {
	if utt == nil {
		return nil
	}
	now := time.Now()
	utt.Status = UtterancePlaying
	utt.StartAt = now
	if strings.TrimSpace(utt.Content) == "" {
		utt.Status = UtterancePlayed
		utt.DurationSeconds = 0
		effects := []effect{
			logRecordEffect{record: buildAIUtteranceLogRecord(utt)},
			markActivityEffect{at: now},
		}
		return append(effects, c.advanceTimelineEffects()...)
	}

	utt.ResponseID = c.state.nextID("resp")
	c.state.current = utt
	c.state.utteranceByResponseID[utt.ResponseID] = utt

	return []effect{
		logRecordEffect{record: buildAIUtteranceLogRecord(utt)},
		markActivityEffect{at: now},
		emitEventEffect{event: types.Event{
			Kind: types.EventRealtimeOutput,
			Payload: types.OutputLine{
				Role:       "assistant",
				Text:       utt.Content,
				ResponseID: utt.ResponseID,
				Source:     utteranceSource(utt),
			},
		}},
		emitEventEffect{event: types.Event{
			Kind: types.EventRealtimeOutput,
			Payload: types.OutputLine{
				Role:       "assistant",
				ResponseID: utt.ResponseID,
				Final:      true,
				Source:     utteranceSource(utt),
			},
		}},
	}
}

func buildAIUtteranceLogRecord(utt *Utterance) logRecord {
	return logRecord{
		Speaker:    "ai",
		Text:       utt.Content,
		ResponseID: utt.ResponseID,
		Source:     utteranceSource(utt),
	}
}
