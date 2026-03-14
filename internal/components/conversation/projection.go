package conversation

import (
	"time"

	"smart-speaker/internal/state"
	types "smart-speaker/internal/types"
)

type projection struct{}

func newProjection() *projection {
	return &projection{}
}

func (p *projection) UpdateConversation(messages []types.ChatMessage) {
	state.SetConversationMessages(messages)
}

func (p *projection) ClearConversation() {
	state.ClearConversationMessages()
}

func (p *projection) MarkActivity(t time.Time) {
	state.SetLastActivityAt(t)
}
