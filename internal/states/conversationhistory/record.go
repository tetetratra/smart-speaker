package conversationhistory

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func NewRecord(req types.ConversationCommitRequest, currentGeneration types.GenerationID) types.ConversationRecord {
	now := time.Now()
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = types.RoleUser
	}
	record := types.ConversationRecord{
		ID:           fmt.Sprintf("%d-%s-%d", now.UnixNano(), role, req.GenerationID),
		Role:         role,
		Text:         strings.TrimSpace(req.Text),
		GenerationID: req.GenerationID,
		Source:       strings.TrimSpace(req.Source),
		Metadata:     map[string]any{},
		CreatedAt:    now,
	}
	if req.ToolCall != nil {
		record.Role = types.RoleToolCall
		record.Text = strings.TrimSpace(string(req.ToolCall.Arguments))
		record.GenerationID = req.ToolCall.GenerationID
		record.Source = strings.TrimSpace(req.ToolCall.Name)
		record.Metadata["tool_call_id"] = strings.TrimSpace(req.ToolCall.ToolCallID)
		record.Metadata["tool_name"] = strings.TrimSpace(req.ToolCall.Name)
		return record
	}
	if req.ToolResult != nil {
		record.Role = types.RoleToolResult
		record.Text = strings.TrimSpace(string(req.ToolResult.Output))
		record.GenerationID = req.ToolResult.GenerationID
		record.Source = strings.TrimSpace(req.ToolResult.Name)
		record.Metadata["tool_call_id"] = strings.TrimSpace(req.ToolResult.ToolCallID)
		record.Metadata["tool_name"] = strings.TrimSpace(req.ToolResult.Name)
		record.Metadata["current_generation_id"] = uint64(currentGeneration)
		record.Metadata["stale"] = req.ToolResult.GenerationID != currentGeneration
	}
	return record
}

func ToChatMessages(records []types.ConversationRecord) []types.ChatMessage {
	messages := make([]types.ChatMessage, 0, len(records))
	for _, rec := range records {
		role := strings.TrimSpace(rec.Role)
		content := strings.TrimSpace(rec.Text)
		if role == "" || content == "" {
			continue
		}
		switch role {
		case types.RoleUser, types.RoleAgent:
			content = formatMessageContent(rec)
		case types.RoleToolCall:
			content = formatToolCallContent(rec)
		case types.RoleToolResult:
			content = formatToolResultContent(rec)
		}
		messages = append(messages, types.ChatMessage{Role: role, Content: content})
	}
	return messages
}

func formatMessageContent(rec types.ConversationRecord) string {
	payload := map[string]any{
		"type":          "message",
		"text":          rec.Text,
		"generation_id": uint64(rec.GenerationID),
	}
	if strings.TrimSpace(rec.Source) != "" {
		payload["source"] = strings.TrimSpace(rec.Source)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return rec.Text
	}
	return string(encoded)
}

func formatToolCallContent(rec types.ConversationRecord) string {
	payload := map[string]any{
		"type":          "tool_call",
		"tool_name":     rec.Source,
		"generation_id": uint64(rec.GenerationID),
		"arguments":     json.RawMessage(rec.Text),
	}
	for key, value := range rec.Metadata {
		payload[key] = value
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return rec.Text
	}
	return string(encoded)
}

func formatToolResultContent(rec types.ConversationRecord) string {
	payload := map[string]any{
		"type":          "tool_result",
		"tool_name":     rec.Source,
		"generation_id": uint64(rec.GenerationID),
		"output":        json.RawMessage(rec.Text),
	}
	for key, value := range rec.Metadata {
		payload[key] = value
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return rec.Text
	}
	return string(encoded)
}
