package conversation

func defaultConversationRules() []Rule {
	return []Rule{
		humanTextRule{},
		responsesStreamRule{},
		toolResponseRule{},
		ttsEndRule{},
		timerElapsedRule{},
	}
}
