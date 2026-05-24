package conversationhistory

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	types "smart-speaker/internal/types"
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
	if req.ToolResult != nil {
		record.Role = types.RoleTool
		record.Text = string(req.ToolResult.Output)
		record.GenerationID = req.ToolResult.GenerationID
		record.Source = req.ToolResult.Name
		record.Metadata["tool_call_id"] = req.ToolResult.ToolCallID
		record.Metadata["tool_name"] = req.ToolResult.Name
		record.Metadata["current_generation_id"] = uint64(currentGeneration)
		record.Metadata["stale"] = req.ToolResult.GenerationID != currentGeneration
	}
	if req.ToolCall != nil {
		record.Role = types.RoleToolCall
		record.Text = string(toolCallArgs(req.ToolCall.Arguments))
		record.GenerationID = req.ToolCall.GenerationID
		record.Source = req.ToolCall.Name
		record.Metadata["tool_call_id"] = req.ToolCall.ToolCallID
		record.Metadata["tool_name"] = req.ToolCall.Name
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
		case types.RoleTool:
			content = formatToolContent(rec)
			role = types.RoleUser
		case types.RoleToolCall:
			content = formatToolCallContent(rec)
			role = types.RoleAssistant
		default:
			content = formatConversationContent(role, content)
		}
		messages = append(messages, types.ChatMessage{Role: role, Content: content})
	}
	return messages
}

func formatConversationContent(role string, content string) string {
	switch role {
	case types.RoleUser:
		return "ユーザー: " + content
	case types.RoleAssistant:
		return "あなた: " + content
	default:
		return content
	}
}

func formatToolContent(rec types.ConversationRecord) string {
	payload := toolPayload(rec)
	for key, value := range rec.Metadata {
		if key == "tool_name" {
			continue
		}
		payload[key] = value
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		encoded, err = json.Marshal(toolPayload(rec))
		if err != nil {
			return "ツール結果: " + rec.Text
		}
	}
	return "ツール結果: " + string(encoded)
}

func formatToolCallContent(rec types.ConversationRecord) string {
	payload := map[string]any{
		"type":          "tool_call",
		"tool_name":     toolName(rec),
		"generation_id": uint64(rec.GenerationID),
		"args":          toolOutput(rec.Text),
	}
	for key, value := range rec.Metadata {
		if key == "tool_name" {
			continue
		}
		payload[key] = value
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		fallback := map[string]any{
			"type":          "tool_call",
			"tool_name":     toolName(rec),
			"generation_id": uint64(rec.GenerationID),
			"args":          toolOutput(rec.Text),
		}
		encoded, err = json.Marshal(fallback)
		if err != nil {
			return "ツール呼び出し: " + rec.Text
		}
	}
	return "ツール呼び出し: " + string(encoded)
}

func toolPayload(rec types.ConversationRecord) map[string]any {
	return map[string]any{
		"type":          "tool_result",
		"tool_name":     toolName(rec),
		"generation_id": uint64(rec.GenerationID),
		"output":        toolOutput(rec.Text),
	}
}

func toolName(rec types.ConversationRecord) string {
	if name, ok := rec.Metadata["tool_name"].(string); ok {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimSpace(rec.Source)
}

func toolOutput(text string) any {
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(text), &raw); err == nil {
		return raw
	}
	return text
}

func toolCallArgs(args json.RawMessage) json.RawMessage {
	if len(args) == 0 {
		return json.RawMessage(`{}`)
	}
	return args
}
