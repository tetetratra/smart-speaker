package conversation

import (
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
	if name == "write_diary" {
		return effects, true
	}
	output := strings.TrimSpace(string(resp.Output))
	if output == "" {
		return effects, true
	}
	content := "ツール実行結果(" + name + "): " + output
	core.state.appendUtterance(&Utterance{
		ID:         core.state.nextID("tool"),
		Speaker:    SpeakerTool,
		StartAt:    time.Now(),
		Content:    content,
		Status:     UtterancePlayed,
		ResponseID: strings.TrimSpace(resp.ResponseID),
	})
	effects = append(effects, logRecordEffect{
		record: logRecord{
			Speaker:    "tool",
			Text:       content,
			ResponseID: strings.TrimSpace(resp.ResponseID),
			Source:     name,
		},
	})
	effects = append(effects, emitConversationSnapshotEffect(core.state.buildConversationMessages()))
	return effects, true
}
