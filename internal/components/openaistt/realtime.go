package openaistt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"nhooyr.io/websocket"
)

const defaultRealtimeEndpoint = "wss://api.openai.com/v1/realtime"

type realtimeConn interface {
	Write(ctx context.Context, typ websocket.MessageType, data []byte) error
	Read(ctx context.Context) (websocket.MessageType, []byte, error)
	Close(status websocket.StatusCode, reason string) error
}

type realtimeDialer interface {
	Dial(ctx context.Context, cfg Config) (realtimeConn, error)
}

type websocketDialer struct{}

func (websocketDialer) Dial(ctx context.Context, cfg Config) (realtimeConn, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = defaultRealtimeEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}
	q := u.Query()
	q.Set("model", modelOrDefault(cfg.Model))
	u.RawQuery = q.Encode()

	header := http.Header{}
	header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
	conn, _, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func sendSessionUpdate(ctx context.Context, conn realtimeConn, cfg Config) error {
	transcription := map[string]any{
		"model": modelOrDefault(cfg.Model),
	}
	if keywords := transcriptionKeywords(cfg.Phrases); len(keywords) > 0 {
		transcription["keywords"] = keywords
	}
	return writeJSON(ctx, conn, map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"type": "transcription",
			"audio": map[string]any{
				"input": map[string]any{
					"format": map[string]any{
						"type": "audio/pcm",
						"rate": openAIInputSampleRate,
					},
					"transcription":  transcription,
					"turn_detection": nil,
				},
			},
		},
	})
}

func sendAudioAppend(ctx context.Context, conn realtimeConn, pcm []byte) error {
	if len(pcm) == 0 {
		return nil
	}
	for start := 0; start < len(pcm); start += openAIChunkBytes {
		end := min(start+openAIChunkBytes, len(pcm))
		if (end-start)%2 != 0 {
			end--
		}
		if end <= start {
			continue
		}
		if err := writeJSON(ctx, conn, map[string]any{
			"type":  "input_audio_buffer.append",
			"audio": base64.StdEncoding.EncodeToString(pcm[start:end]),
		}); err != nil {
			return err
		}
	}
	return nil
}

func sendAudioCommit(ctx context.Context, conn realtimeConn) error {
	return writeJSON(ctx, conn, map[string]any{"type": "input_audio_buffer.commit"})
}

func writeJSON(ctx context.Context, conn realtimeConn, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

func transcriptionKeywords(phrases []string) []string {
	out := make([]string, 0, len(phrases))
	seen := make(map[string]struct{}, len(phrases))
	for _, phrase := range phrases {
		trimmed := strings.TrimSpace(phrase)
		if trimmed == "" || strings.ContainsAny(trimmed, "<>\r\n") {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func modelOrDefault(model string) string {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return defaultModel
	}
	return trimmed
}
