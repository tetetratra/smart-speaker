package conversation

import (
	"encoding/json"
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

func (c *conversationCore) buildResponseRequestEffect(messages []types.ChatMessage) []effect {
	if len(messages) == 0 {
		return nil
	}
	reqID := c.state.nextID("req")
	generationID := c.state.currentGeneration()
	c.state.pendingRequestID = reqID
	c.state.pendingRequestStreaming = false
	c.state.pendingStreamSpeechStarted = false
	c.state.pendingStreamFailed = false
	c.state.pendingStreamToolSeen = false
	c.state.requestGeneration[reqID] = generationID
	c.state.clearPendingStreamLines()
	return []effect{requestResponseEffect{
		requestID:    reqID,
		generationID: generationID,
		messages:     messages,
	}}
}

func (c *conversationCore) retryInvalidResponseEffects(invalidRaw string) []effect {
	if c.state.invalidResponseRetries >= maxInvalidResponseRetries {
		return []effect{runtimeLogEffect{
			message: "conversation: invalid response retry exhausted (1/1)",
		}}
	}
	messages := c.state.buildConversationMessages()
	if len(messages) == 0 {
		return nil
	}
	messages = append([]types.ChatMessage{{
		Role:    "system",
		Content: buildInvalidResponseRetryHint(invalidRaw),
	}}, messages...)
	c.state.invalidResponseRetries++
	effects := c.buildResponseRequestEffect(messages)
	effects = append(effects, runtimeLogEffect{
		message: "conversation: retrying due to invalid response (1/1)",
	})
	return effects
}

func buildInvalidResponseRetryHint(invalidRaw string) string {
	raw := strings.TrimSpace(invalidRaw)
	if raw == "" {
		raw = "(空)"
	}
	quoted, err := json.Marshal(raw)
	if err != nil {
		quoted = []byte(`"(エスケープ失敗)"`)
	}
	return importantRetryPrefix +
		"直近のレスポンスは契約違反でした。必ず 1 行 1 JSON object の NDJSON だけを返してください。各行は " +
		"{\"type\":\"speech\",\"text\":\"文字列\"} / {\"type\":\"wait\",\"sec\":整数} " +
		"/ {\"type\":\"tool\",\"name\":\"ツール名\",\"args\":{...}} のいずれかにしてください。" +
		"tool は末尾に最大1件だけ置けます。直近の違反レスポンス文字列は " + string(quoted) + " です。"
}

func (c *conversationCore) advanceTimelineEffects() []effect {
	if c.state.current != nil {
		return nil
	}
	for c.state.pendingTimelineIdx < len(c.state.pendingTimeline) {
		seg := c.state.pendingTimeline[c.state.pendingTimelineIdx]
		c.state.pendingTimelineIdx++
		switch seg.Type {
		case "wait":
			delay := time.Duration(seg.WaitSec) * time.Second
			if delay > 0 {
				c.state.pendingTimelineTimerWaiting = true
				return []effect{startTimerEffect{duration: delay}}
			}
		case "speech":
			utt := &Utterance{
				ID:           c.state.nextID("ai"),
				Speaker:      SpeakerAI,
				Content:      seg.Text,
				Status:       UtteranceUnplayed,
				GenerationID: c.state.currentGeneration(),
			}
			c.state.appendUtterance(utt)
			return c.playUtteranceEffects(utt)
		case "tool":
			if seg.Tool == nil {
				continue
			}
			return []effect{emitEventEffect{event: types.Event{
				Kind: types.EventToolRequest,
				Payload: types.ToolRequest{
					ToolCallID:   c.state.nextID("tool_call"),
					Name:         seg.Tool.Name,
					Arguments:    seg.Tool.Args,
					GenerationID: c.state.currentGeneration(),
				},
			}}}
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
		}
		return append(effects, c.advanceTimelineEffects()...)
	}

	utt.ResponseID = c.state.nextID("resp")
	c.state.current = utt
	c.state.utteranceByResponseID[utt.ResponseID] = utt

	return []effect{
		logRecordEffect{record: buildAIUtteranceLogRecord(utt)},
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

func utteranceSource(_ *Utterance) string {
	return "conversation"
}
