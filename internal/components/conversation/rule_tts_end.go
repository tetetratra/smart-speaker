package conversation

import "strings"

type ttsEndRule struct{}

func (ttsEndRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	s, ok := sig.(ttsEndSignal)
	if !ok {
		return nil, false
	}
	respID := strings.TrimSpace(s.event.ResponseID)
	if respID == "" {
		return nil, true
	}
	utt := core.state.utteranceByResponseID[respID]
	if utt == nil {
		return nil, true
	}
	if utt.Status == UtterancePlaying {
		utt.Status = UtterancePlayed
		utt.DurationSeconds = s.event.DurationSeconds
	}
	delete(core.state.utteranceByResponseID, respID)
	if core.state.current == utt {
		core.state.current = nil
	}
	effects := []effect{}
	if !core.state.hasPendingSpeech() {
		core.state.clearPendingTimeline()
		effects = append(effects, emitConversationSnapshotEffect(core.state.buildConversationMessages()))
		return effects, true
	}
	waitSec := core.state.consumeLeadingWaitSeconds()
	if !core.state.hasPendingSpeech() {
		core.state.clearPendingTimeline()
		effects = append(effects, emitConversationSnapshotEffect(core.state.buildConversationMessages()))
		return effects, true
	}
	effects = append(effects,
		startTimerEffect{duration: estimateWaitDuration(s.event, waitSec)},
		emitConversationSnapshotEffect(core.state.buildConversationMessages()),
	)
	return effects, true
}
