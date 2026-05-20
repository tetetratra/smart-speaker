package conversation

func defaultConversationRules() []Rule {
	return []Rule{
		humanTextRule{},
		responsesRule{},
		responsesStreamRule{},
		toolResponseRule{},
		ttsEndRule{},
		timerElapsedRule{},
	}
}
