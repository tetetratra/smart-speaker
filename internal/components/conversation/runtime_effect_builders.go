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
