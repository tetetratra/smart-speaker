package receiver

import (
	"encoding/json"
	"log"

	types "smart-speaker/internal/types"
)

type wsMessage map[string]any

const debugPrintMsgType = false
const debugDumpResponses = false

type MessageParser struct {
	responseDeltaSeen map[string]bool
}

func NewMessageParser() *MessageParser {
	return &MessageParser{responseDeltaSeen: make(map[string]bool)}
}

func (p *MessageParser) Parse(msg wsMessage) []types.Event {
	if debugDumpResponses {
		if data, err := json.MarshalIndent(msg, "", "  "); err == nil {
			log.Println(string(data))
		}
	}
	if debugPrintMsgType {
		log.Println(msg["type"].(string))
	}
	switch msgType := msg["type"].(string); msgType {
	// conversation.item.input_audio_transcription.delta: 音声入力の途中文字起こし。
	case "conversation.item.input_audio_transcription.delta":
		if transcript, ok := msg["transcript"].(string); ok {
			return []types.Event{{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "user", Text: transcript}}}
		}
	// conversation.item.input_audio_transcription.completed: 音声入力の文字起こし完了時。
	case "conversation.item.input_audio_transcription.completed":
		if transcript, ok := msg["transcript"].(string); ok {
			return []types.Event{{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "user", Text: transcript}}}
		}
	// response.output_text.delta: アシスタントのテキスト応答（途中の差分）。
	case "response.output_text.delta":
		if delta, ok := msg["delta"].(string); ok {
			if id, ok := msg["response_id"].(string); ok && id != "" {
				p.responseDeltaSeen[id] = true
			}
			return []types.Event{{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "assistant", Text: delta}}}
		}
	// response.output_text: アシスタントのテキスト応答（まとめて）。
	case "response.output_text":
		if text, ok := msg["text"].(string); ok {
			if id, ok := msg["response_id"].(string); ok && id != "" {
				p.responseDeltaSeen[id] = true
			}
			return []types.Event{{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "assistant", Text: text}}}
		}
	// response.output_audio.delta: アシスタント音声応答の差分（Base64等）。
	case "response.output_audio.delta":
		if delta, ok := msg["delta"].(string); ok && delta != "" {
			return []types.Event{{Kind: types.EventRealtimeAudio, Payload: types.OutputAudio{Role: "assistant", Audio: delta}}}
		}
	case "response.audio_transcript.delta":
		if transcript, ok := msg["transcript"].(string); ok {
			return []types.Event{{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "assistant", Text: transcript}}}
		}
	case "response.audio_transcript.done":
		if transcript, ok := msg["transcript"].(string); ok {
			return []types.Event{{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "assistant", Text: transcript}}}
		}
	// response.delta: response.* の複合イベント（途中段階）。
	case "response.delta":
		return p.extractEventsFromResponse(msgType, msg)
	// response.done: レスポンス完了時のまとめ。
	case "response.done":
		return p.extractEventsFromResponse(msgType, msg)
	// error/response.error: APIエラー通知。
	case "error", "response.error":
		if detail, ok := msg["error"].(map[string]any); ok {
			if message, ok := detail["message"].(string); ok {
				return []types.Event{{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: "error", Text: message}}}
			}
		}
	}
	return nil
}

func (p *MessageParser) extractEventsFromResponse(msgType string, msg wsMessage) []types.Event {
	resp, ok := msg["response"].(map[string]any)
	if !ok {
		return nil
	}

	respID, _ := resp["id"].(string)
	if respID != "" && msgType == "response.done" {
		if p.responseDeltaSeen[respID] {
			delete(p.responseDeltaSeen, respID)
			return nil
		}
	}

	output, ok := resp["output"].([]any)
	if !ok {
		return nil
	}

	var events []types.Event
	hasAssistant := false
	for _, entry := range output {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		role, _ := item["role"].(string)
		contentEvents := collectContentEvents(role, item)
		for _, evt := range contentEvents {
			if evt.Kind == types.EventRealtimeOutput {
				if line, ok := evt.Payload.(types.OutputLine); ok && line.Role == "assistant" {
					hasAssistant = true
				}
			}
			if evt.Kind == types.EventRealtimeAudio {
				hasAssistant = true
			}
			events = append(events, evt)
		}
	}
	if respID != "" && msgType == "response.delta" && hasAssistant {
		p.responseDeltaSeen[respID] = true
	}
	return events
}

func collectContentEvents(role string, item map[string]any) []types.Event {
	content, ok := item["content"].([]any)
	if !ok {
		return nil
	}

	var events []types.Event
	for _, part := range content {
		partMap, ok := part.(map[string]any)
		if !ok {
			continue
		}
		switch partMap["type"].(string) {
		case "text":
			text, _ := partMap["text"].(string)
			if text == "" {
				continue
			}
			events = append(events, types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: roleOrDefault(role, "assistant"), Text: text}})
		case "input_text":
			text, _ := partMap["text"].(string)
			if text == "" {
				continue
			}
			events = append(events, types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: roleOrDefault(role, "user"), Text: text}})
		case "output_audio", "audio":
			if audioMap, ok := partMap["audio"].(map[string]any); ok {
				if data, ok := audioMap["data"].([]any); ok {
					for _, chunk := range data {
						if str, ok := chunk.(string); ok && str != "" {
							events = append(events, types.Event{Kind: types.EventRealtimeAudio, Payload: types.OutputAudio{Role: roleOrDefault(role, "assistant"), Audio: str}})
						}
					}
				}
			}
		}
	}
	return events
}

func roleOrDefault(role, fallback string) string {
	if role == "" {
		return fallback
	}
	return role
}
