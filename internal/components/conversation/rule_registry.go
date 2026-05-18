package conversation

func defaultConversationRules() []Rule {
	return []Rule{
		speechStartRule{},
		humanTextRule{},
		responsesRule{},
		responsesStreamRule{},
		toolResponseRule{},
		ttsEndRule{},
		timerElapsedRule{},
	}
}
