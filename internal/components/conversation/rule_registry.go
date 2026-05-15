package conversation

func defaultConversationRules() []Rule {
	return []Rule{
		speechStartRule{},
		humanTextRule{},
		timerFiredRule{},
		responsesRule{},
		responsesStreamRule{},
		toolResponseRule{},
		sessionClearRule{},
		ttsEndRule{},
		timerElapsedRule{},
	}
}
