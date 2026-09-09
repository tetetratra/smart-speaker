package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type ContextProviderConfig struct {
	Memory memoryReader
}

type ContextProvider struct {
	memory memoryReader
}

type memoryReader interface {
	Snapshot() []memorystate.Record
}

func NewContextProvider(cfg ContextProviderConfig) (*ContextProvider, error) {
	if cfg.Memory == nil {
		return nil, fmt.Errorf("memory context provider: memory is required")
	}
	return &ContextProvider{memory: cfg.Memory}, nil
}

func (p *ContextProvider) BuildContext(_ context.Context, _ []types.ConversationRecord) ([]types.ChatMessage, error) {
	return memoryContextMessages(p.memory.Snapshot())
}

func memoryContextMessages(records []memorystate.Record) ([]types.ChatMessage, error) {
	memories := make([]memoryContextItem, 0, len(records))
	for _, record := range records {
		content := strings.TrimSpace(record.Content)
		if content == "" {
			continue
		}
		memories = append(memories, memoryContextItem{Content: content})
	}
	if len(memories) == 0 {
		return nil, nil
	}
	payload := memoryContextPayload{
		Type:     "memory_context",
		Memories: memories,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return []types.ChatMessage{{Role: types.RoleSystem, Content: string(encoded)}}, nil
}

type memoryContextPayload struct {
	Type     string              `json:"type"`
	Memories []memoryContextItem `json:"memories"`
}

type memoryContextItem struct {
	Content string `json:"content"`
}
