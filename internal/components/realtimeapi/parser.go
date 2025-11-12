package realtimeapi

import (
	"encoding/json"
	"log"

	types "smart-speaker/internal/types"
)

const debugDumpResponses = false

func (c *Client) parseMessageForLines(msg wsMessage) []types.OutputLine {
	if debugDumpResponses {
		if data, err := json.MarshalIndent(msg, "", "  "); err == nil {
			log.Println(string(data))
		}
	}
	switch msgType := msg["type"].(string); msgType {
	case "conversation.item.input_audio_transcription.delta":
		if transcript, ok := msg["transcript"].(string); ok {
			return []types.OutputLine{{Role: "user", Text: transcript}}
		}
	case "conversation.item.input_audio_transcription.completed":
		if transcript, ok := msg["transcript"].(string); ok {
			return []types.OutputLine{{Role: "user", Text: transcript}}
		}
	case "response.output_text.delta":
		if delta, ok := msg["delta"].(string); ok {
			if id, ok := msg["response_id"].(string); ok && id != "" {
				c.responseDeltaSeen[id] = true
			}
			return []types.OutputLine{{Role: "assistant", Text: delta}}
		}
	case "response.output_text":
		if text, ok := msg["text"].(string); ok {
			if id, ok := msg["response_id"].(string); ok && id != "" {
				c.responseDeltaSeen[id] = true
			}
			return []types.OutputLine{{Role: "assistant", Text: text}}
		}
	case "response.delta":
		return c.extractLinesFromResponse(msgType, msg)
	case "response.done":
		return c.extractLinesFromResponse(msgType, msg)
	case "error", "response.error":
		if detail, ok := msg["error"].(map[string]any); ok {
			if message, ok := detail["message"].(string); ok {
				return []types.OutputLine{{Role: "error", Text: message}}
			}
		}
	}
	return nil
}

func (c *Client) extractLinesFromResponse(msgType string, msg wsMessage) []types.OutputLine {
	resp, ok := msg["response"].(map[string]any)
	if !ok {
		return nil
	}

	respID, _ := resp["id"].(string)
	if respID != "" && msgType == "response.done" {
		if c.responseDeltaSeen[respID] {
			delete(c.responseDeltaSeen, respID)
			return nil
		}
	}

	output, ok := resp["output"].([]any)
	if !ok {
		return nil
	}

	var lines []types.OutputLine
	hasAssistant := false
	for _, entry := range output {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		role, _ := item["role"].(string)
		contentLines := collectContentLines(role, item)
		for _, line := range contentLines {
			if line.Role == "assistant" {
				hasAssistant = true
			}
			lines = append(lines, line)
		}
	}
	if respID != "" && msgType == "response.delta" && hasAssistant {
		c.responseDeltaSeen[respID] = true
	}
	return lines
}

func collectContentLines(role string, item map[string]any) []types.OutputLine {
	content, ok := item["content"].([]any)
	if !ok {
		return nil
	}

	var lines []types.OutputLine
	for _, part := range content {
		partMap, ok := part.(map[string]any)
		if !ok {
			continue
		}
		text, _ := partMap["text"].(string)
		if text == "" {
			continue
		}
		switch partMap["type"].(string) {
		case "text":
			lines = append(lines, types.OutputLine{Role: roleOrDefault(role, "assistant"), Text: text})
		case "input_text":
			lines = append(lines, types.OutputLine{Role: roleOrDefault(role, "user"), Text: text})
		}
	}
	return lines
}

func roleOrDefault(role, fallback string) string {
	if role == "" {
		return fallback
	}
	return role
}
