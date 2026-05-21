package conversation

import (
	"fmt"
	"strings"
	"time"
)

type toolResponseRule struct{}

func (toolResponseRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	s, ok := sig.(toolResponseSignal)
	if !ok {
		return nil, false
	}
	resp := s.response
	name := strings.TrimSpace(resp.Name)
	if name == "" {
		name = "unknown_tool"
	}
	var effects []effect
	output := strings.TrimSpace(string(resp.Output))
	if output == "" {
		return effects, true
	}
	core.state.clearPendingTimeline()
	core.state.cancelPendingRequest()
	core.state.invalidResponseRetries = 0
	effects = append(effects, stopTimerEffect{})

	stale := resp.GenerationID != 0 && resp.GenerationID != core.state.currentGeneration()
	content := fmt.Sprintf(
		"ツール実行結果(name=%s, generation_id=%d, current_generation_id=%d, stale=%t): %s",
		name,
		resp.GenerationID,
		core.state.currentGeneration(),
		stale,
		output,
	)
	core.state.appendUtterance(&Utterance{
		ID:           core.state.nextID("tool"),
		Speaker:      SpeakerTool,
		StartAt:      time.Now(),
		Content:      content,
		Status:       UtterancePlayed,
		ResponseID:   strings.TrimSpace(resp.ResponseID),
		GenerationID: resp.GenerationID,
	})
	effects = append(effects, logRecordEffect{
		record: logRecord{
			Speaker:    "tool",
			Text:       content,
			ResponseID: strings.TrimSpace(resp.ResponseID),
			Source:     name,
		},
	})
	messages := core.state.buildConversationMessages()
	effects = append(effects, core.buildResponseRequestEffect(messages)...)
	return effects, true
}
