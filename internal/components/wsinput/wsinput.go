package wsinput

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"

	"nhooyr.io/websocket"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

// WSInput はブラウザからの WebSocket 音声入力を受け取り EventAudioChunk を下流に流す。
type WSInput struct {
	downstream chan types.Event
	server     *http.Server
	clients    *sync.Map
	once       sync.Once
}

// Message はクライアントから届く JSON メッセージ。
// 例: {"type":"audio.append","audio":"<base64 pcm16>"}
type Message struct {
	Type  string `json:"type"`
	Audio string `json:"audio,omitempty"`
}

// NewStage は指定アドレスで WS サーバーを立て、受信音声を downstream に流す Stage を返す。
func NewStage(server *http.Server, mux *http.ServeMux, clients *sync.Map) *graph.Stage {
	w := &WSInput{
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		server:     server,
		clients:    clients,
	}
	mux.HandleFunc("/ws/audio", w.handleWS)
	return &graph.Stage{
		Downstream: w.downstream,
		Run:        w.run,
		CloseFn:    w.close,
	}
}

func (w *WSInput) run(ctx context.Context) {
	go func() {
		if err := w.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("wsinput listen error: %v", err)
		}
	}()
}

func (w *WSInput) close() error {
	var err error
	w.once.Do(func() {
		close(w.downstream)
		if w.server != nil {
			err = w.server.Close()
		}
		if w.clients != nil {
			w.clients.Range(func(key, _ any) bool {
				conn := key.(*websocket.Conn)
				conn.Close(websocket.StatusNormalClosure, "bye")
				return true
			})
		}
	})
	return err
}

func (w *WSInput) handleWS(rw http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(rw, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("ws accept error: %v", err)
		return
	}
	w.clients.Store(c, struct{}{})
	defer c.Close(websocket.StatusNormalClosure, "bye")
	defer w.clients.Delete(c)

	for {
		_, data, err := c.Read(r.Context())
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("wsinput read error: %v", err)
			}
			return
		}
		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("wsinput json error: %v", err)
			continue
		}
		if msg.Type != "audio.append" || msg.Audio == "" {
			continue
		}
		evt := types.Event{Kind: types.EventAudioChunk, Payload: types.AudioChunk(msg.Audio)}
		select {
		case w.downstream <- evt:
		case <-r.Context().Done():
			return
		}
	}
}
