package conversation

import "time"

type humanTextRule struct{}

func (humanTextRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	s, ok := sig.(humanTextSignal)
	if !ok {
		return nil, false
	}
	core.state.nextGeneration()
	core.state.clearPendingTimeline()
	core.state.cancelPendingRequest()
	core.state.invalidResponseRetries = 0
	effects := []effect{stopTimerEffect{}}
	core.state.appendUtterance(&Utterance{
		ID:           core.state.nextID("human"),
		Speaker:      SpeakerHuman,
		StartAt:      time.Now(),
		Content:      s.text,
		Status:       UtterancePlayed,
		GenerationID: core.state.currentGeneration(),
	})
	effects = append(effects, logRecordEffect{
		record: logRecord{
			Speaker: "human",
			Text:    s.text,
		},
	})
	messages := core.state.buildConversationMessages()
	effects = append(effects, core.buildResponseRequestEffect(messages)...)
	return effects, true
}
