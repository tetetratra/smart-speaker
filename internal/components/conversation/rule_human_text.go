package conversation

import "time"

type humanTextRule struct{}

func (humanTextRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	s, ok := sig.(humanTextSignal)
	if !ok {
		return nil, false
	}
	now := time.Now()
	decision := core.decideReaction(reactionInput{
		Text:   s.text,
		Source: s.source,
		Now:    now,
	})
	effects := []effect{
		emitConversationReactionEffect(decision),
		logRecordEffect{record: reactionLogRecord(decision)},
	}
	switch decision.Level {
	case reactionIgnore:
		return effects, true
	case reactionSilentObserve:
		core.state.addObservedMonologue(observedMonologue{
			Text:    decision.Text,
			Source:  decision.Source,
			Reasons: append([]string(nil), decision.Reasons...),
			Score:   decision.Score,
			At:      decision.At,
		})
		return effects, true
	}
	effects = append(effects, core.interruptCurrentConversationEffects()...)
	core.state.appendUtterance(&Utterance{
		ID:      core.state.nextID("human"),
		Speaker: SpeakerHuman,
		StartAt: now,
		Content: s.text,
		Status:  UtterancePlayed,
	})
	effects = append(effects, logRecordEffect{
		record: humanLogRecord(s.text, decision.Source, decision),
	})
	messages := core.state.buildConversationMessages()
	requestMessages := injectObservedMonologueContext(messages, core.state.consumeRecentObservedMonologues(now))
	effects = append(effects,
		emitConversationActivityEffect(now, "human_turn_committed"),
		emitConversationSnapshotEffect(messages),
	)
	effects = append(effects, core.buildResponseRequestEffect(requestMessages, nil)...)
	return effects, true
}
