package conversation

type sessionClearRule struct{}

func (sessionClearRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	if _, ok := sig.(sessionClearSignal); !ok {
		return nil, false
	}
	effects := core.interruptCurrentConversationEffects()
	core.state.resetConversation()
	effects = append(effects, emitConversationSnapshotEffect(nil))
	return effects, true
}
