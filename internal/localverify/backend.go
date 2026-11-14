// Package localverify は外部APIを呼ばずに会話経路を検証するための偽サービス。
package localverify

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"nhooyr.io/websocket"
	"strings"
	"sync"
	"time"
)

type Backend struct {
	ctx        context.Context
	mu         sync.Mutex
	conn       *websocket.Conn
	count      int
	text, user string
}

func New(ctx context.Context) (*Backend, http.Handler) {
	b := &Backend{ctx: ctx}
	mux := http.NewServeMux()
	mux.HandleFunc("/realtime", b.realtime)
	mux.HandleFunc("/audio_query", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"speedScale":1,"outputSamplingRate":24000,"outputStereo":false}`))
	})
	mux.HandleFunc("/synthesis", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Write(toneWAV())
	})
	mux.HandleFunc("/debug/utterance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		var input struct{ Text string }
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil || strings.TrimSpace(input.Text) == "" {
			http.Error(w, "textが必要", 400)
			return
		}
		if err := b.Say(input.Text); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		w.WriteHeader(204)
	})
	return b, mux
}
func (b *Backend) Say(text string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.conn == nil {
		return fmt.Errorf("Realtime接続がまだありません")
	}
	b.count++
	b.text = text
	b.user = fmt.Sprintf("user%d", b.count)
	for _, event := range []any{
		map[string]any{"type": "input_audio_buffer.speech_started", "item_id": b.user},
		map[string]any{"type": "input_audio_buffer.speech_stopped", "item_id": b.user},
		map[string]any{"type": "input_audio_buffer.committed", "item_id": b.user},
		map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": b.user, "transcript": text},
	} {
		if err := b.send(event); err != nil {
			return err
		}
	}
	return nil
}
func (b *Backend) send(v any) error {
	ctx, cancel := context.WithTimeout(b.ctx, 3*time.Second)
	defer cancel()
	raw, _ := json.Marshal(v)
	return b.conn.Write(ctx, websocket.MessageText, raw)
}
func (b *Backend) realtime(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "")
	// ツールの一覧を含むセッション設定は既定の上限（32KiB）を超えるので、本物の API と同じく大きめに受け付ける。
	c.SetReadLimit(1 << 20)
	b.mu.Lock()
	b.conn = c
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		if b.conn == c {
			b.conn = nil
		}
		b.mu.Unlock()
	}()
	for {
		_, raw, err := c.Read(b.ctx)
		if err != nil {
			return
		}
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			return
		}
		b.mu.Lock()
		if b.conn != c {
			b.mu.Unlock()
			return
		}
		switch m["type"] {
		case "session.update":
			b.send(map[string]any{"type": "session.updated"})
		case "conversation.item.delete":
			b.send(map[string]any{"type": "conversation.item.deleted", "item_id": m["item_id"]})
		case "conversation.item.create":
			b.send(map[string]any{"type": "conversation.item.added", "item": m["item"], "previous_item_id": m["previous_item_id"]})
		case "response.create":
			responseID, itemID := fmt.Sprintf("response%d", b.count), fmt.Sprintf("assistant%d", b.count)
			metadata := m["response"].(map[string]any)["metadata"]
			text := "確認しました。\n" + b.text + "。続けて話してください。"
			for _, event := range []any{
				map[string]any{"type": "response.created", "response": map[string]any{"id": responseID, "metadata": metadata}},
				map[string]any{"type": "conversation.item.added", "previous_item_id": b.user, "item": map[string]any{"id": itemID}},
				map[string]any{"type": "response.output_text.delta", "response_id": responseID, "item_id": itemID, "delta": text},
				map[string]any{"type": "response.output_text.done", "response_id": responseID, "item_id": itemID, "text": text},
				map[string]any{"type": "response.done", "response": map[string]any{"id": responseID, "status": "completed"}},
			} {
				if err = b.send(event); err != nil {
					break
				}
			}
		}
		b.mu.Unlock()
		if err != nil {
			return
		}
	}
}
func toneWAV() []byte {
	// 一文につき約1秒の合成音。実際のVOICEVOX品質はこの検証の対象外。
	pcm := make([]byte, 48000)
	for i := 0; i < len(pcm)/2; i++ {
		sample := int16(math.Sin(float64(i)*2*math.Pi*440/24000) * 1500)
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(sample))
	}
	var out bytes.Buffer
	out.WriteString("RIFF")
	binary.Write(&out, binary.LittleEndian, uint32(len(pcm)+36))
	out.WriteString("WAVEfmt ")
	for _, v := range []any{uint32(16), uint16(1), uint16(1), uint32(24000), uint32(48000), uint16(2), uint16(16)} {
		binary.Write(&out, binary.LittleEndian, v)
	}
	out.WriteString("data")
	binary.Write(&out, binary.LittleEndian, uint32(len(pcm)))
	out.Write(pcm)
	return out.Bytes()
}
