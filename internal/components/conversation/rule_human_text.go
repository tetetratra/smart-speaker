package conversation

import "time"

type humanTextRule struct{}

func (humanTextRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	s, ok := sig.(humanTextSignal)
	if !ok {
		return nil, false
	}
	effects := core.interruptCurrentConversationEffects()
	core.state.appendUtterance(&Utterance{
		ID:      core.state.nextID("human"),
		Speaker: SpeakerHuman,
		StartAt: time.Now(),
		Content: s.text,
		Status:  UtterancePlayed,
	})
	effects = append(effects, logRecordEffect{
		record: logRecord{
			Speaker: "human",
			Text:    s.text,
		},
	})
	messages := core.state.buildConversationMessages()
	effects = append(effects,
		emitConversationActivityEffect(time.Now(), "human_turn_committed"),
		emitConversationSnapshotEffect(messages),
	)
	effects = append(effects, core.buildResponseRequestEffect(messages, nil)...)
	return effects, true
}
