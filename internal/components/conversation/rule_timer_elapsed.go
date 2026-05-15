package conversation

type timerElapsedRule struct{}

func (timerElapsedRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	if _, ok := sig.(timerElapsedSignal); !ok {
		return nil, false
	}
	core.state.pendingTimelineTimerWaiting = false
	return core.advanceTimelineEffects(), true
}
