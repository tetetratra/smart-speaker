package conversation

func defaultConversationRules() []Rule {
	return []Rule{
		speechStartRule{},
		humanTextRule{},
		responsesRule{},
		responsesStreamRule{},
		toolResponseRule{},
		sessionClearRule{},
		ttsEndRule{},
		timerElapsedRule{},
	}
}
