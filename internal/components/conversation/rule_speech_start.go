package conversation

type speechStartRule struct{}

func (speechStartRule) Apply(core *conversationCore, sig signal) ([]effect, bool) {
	if _, ok := sig.(speechStartSignal); !ok {
		return nil, false
	}
	return core.interruptCurrentConversationEffects(), true
}
