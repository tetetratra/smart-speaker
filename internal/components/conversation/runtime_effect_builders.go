package conversation

import (
	"time"

	types "smart-speaker/internal/types"
)

func emitConversationSnapshotEffect(messages []types.ChatMessage) emitEventEffect {
	cloned := make([]types.ChatMessage, len(messages))
	copy(cloned, messages)
	return emitEventEffect{
		event: types.Event{
			Kind: types.EventConversationSnapshotUpdated,
			Payload: types.ConversationSnapshot{
				Messages: cloned,
			},
		},
	}
}

func emitConversationActivityEffect(at time.Time, source string) emitEventEffect {
	return emitEventEffect{
		event: types.Event{
			Kind: types.EventConversationActivity,
			Payload: types.ConversationActivity{
				At:     at,
				Source: source,
			},
		},
	}
}

func emitConversationReactionEffect(decision reactionDecision) emitEventEffect {
	reasons := make([]string, len(decision.Reasons))
	copy(reasons, decision.Reasons)
	return emitEventEffect{
		event: types.Event{
			Kind: types.EventConversationReaction,
			Payload: types.ConversationReaction{
				At:          decision.At,
				Text:        decision.Text,
				Source:      decision.Source,
				Level:       string(decision.Level),
				Reasons:     reasons,
				Score:       decision.Score,
				PassedToLLM: decision.Level == reactionVoiceReply,
			},
		},
	}
}
