package conversation

func defaultConversationRules() []Rule {
	return []Rule{
		speechStartRule{},
		humanTextRule{},
		timerFiredRule{},
		responsesRule{},
		toolResponseRule{},
		sessionClearRule{},
		ttsEndRule{},
		timerElapsedRule{},
	}
}
