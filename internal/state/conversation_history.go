package state

import (
	"sync"

	types "smart-speaker/internal/types"
)

var conversationHistory struct {
	mu       sync.RWMutex
	messages []types.ChatMessage
}

// SetConversationMessages stores the current conversation messages in memory.
func SetConversationMessages(messages []types.ChatMessage) {
	conversationHistory.mu.Lock()
	defer conversationHistory.mu.Unlock()
	if len(messages) == 0 {
		conversationHistory.messages = nil
		return
	}
	conversationHistory.messages = make([]types.ChatMessage, len(messages))
	copy(conversationHistory.messages, messages)
}

// GetConversationMessages returns a copy of current conversation messages.
func GetConversationMessages() []types.ChatMessage {
	conversationHistory.mu.RLock()
	defer conversationHistory.mu.RUnlock()
	if len(conversationHistory.messages) == 0 {
		return nil
	}
	out := make([]types.ChatMessage, len(conversationHistory.messages))
	copy(out, conversationHistory.messages)
	return out
}

// ClearConversationMessages clears the stored conversation messages.
func ClearConversationMessages() {
	SetConversationMessages(nil)
}
