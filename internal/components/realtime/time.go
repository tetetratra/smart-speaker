package realtime

import (
	"log"
	"time"

	"github.com/tetetratra/smart-speaker/internal/conversation"
)

// sendNotice は、応答の前に日時の知らせと、この応答だけへの指示を system のメッセージで会話の末尾に足す。
// 指示（instructions）を応答ごとに変えると入力の先頭が変わり、会話全体でキャッシュが効かなくなるため、会話の末尾に足す。
// 古い知らせは消さずに残す。途中の項目を消すと、そこから後ろのキャッシュが効かなくなるため。
func (s *stage) sendNotice(now time.Time, oneOff string) {
	text := conversation.TimeNotice(now)
	if oneOff != "" {
		text += "\n## この応答だけへの指示\n" + oneOff
	}
	s.transport.send(map[string]any{"type": "conversation.item.create", "item": map[string]any{"type": "message", "role": "system", "content": []any{map[string]any{"type": "input_text", "text": text}}}})
}

// usage は response.done に含まれる、応答1回分のトークン数。
type usage struct {
	InputTokens       int `json:"input_tokens"`
	OutputTokens      int `json:"output_tokens"`
	InputTokenDetails struct {
		CachedTokens int `json:"cached_tokens"`
		TextTokens   int `json:"text_tokens"`
		AudioTokens  int `json:"audio_tokens"`
	} `json:"input_token_details"`
}

// logUsage は、キャッシュの効き具合を確かめられるよう、応答ごとのトークン数をログに出す。
func logUsage(u *usage) {
	if u == nil {
		return
	}
	log.Printf("Realtime: 使用量 入力=%d（キャッシュ=%d、テキスト=%d、音声=%d） 出力=%d", u.InputTokens, u.InputTokenDetails.CachedTokens, u.InputTokenDetails.TextTokens, u.InputTokenDetails.AudioTokens, u.OutputTokens)
}
