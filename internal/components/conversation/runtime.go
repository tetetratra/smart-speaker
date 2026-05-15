package conversation

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	types "smart-speaker/internal/types"
)

type logRecord struct {
	Timestamp  string `json:"ts"`
	Speaker    string `json:"speaker"`
	Text       string `json:"text"`
	ResponseID string `json:"response_id,omitempty"`
	Source     string `json:"source,omitempty"`
}

type conversationLogger struct {
	file    *os.File
	writer  *bufio.Writer
	encoder *json.Encoder
}

// 会話ログを書き込むloggerを初期化する。
func newConversationLogger(path string) *conversationLogger {
	writer, encoder, file := openLogWriter(path)
	return &conversationLogger{
		file:    file,
		writer:  writer,
		encoder: encoder,
	}
}

// loggerのbufferとfileを閉じる。
func (l *conversationLogger) Close() error {
	if l == nil {
		return nil
	}
	if l.writer != nil {
		if err := l.writer.Flush(); err != nil {
			log.Printf("conversation: log flush error: %v", err)
		}
	}
	if l.file != nil {
		if err := l.file.Close(); err != nil {
			return err
		}
	}
	return nil
}

// 会話ログ1件をJSON Linesとして書き込む。
func (l *conversationLogger) Write(rec logRecord) {
	if l == nil || l.encoder == nil {
		return
	}
	if rec.Timestamp == "" {
		rec.Timestamp = time.Now().Format(time.RFC3339Nano)
	}
	if err := l.encoder.Encode(rec); err != nil {
		log.Printf("conversation: log encode error: %v", err)
		return
	}
	if err := l.writer.Flush(); err != nil {
		log.Printf("conversation: log flush error: %v", err)
	}
}

// ログ出力先のディレクトリとファイルを開く。
func openLogWriter(path string) (*bufio.Writer, *json.Encoder, *os.File) {
	logPath := strings.TrimSpace(path)
	if logPath == "" {
		return nil, nil, nil
	}
	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("conversation: failed to create log dir: %v", err)
		return nil, nil, nil
	}
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("conversation: failed to open log file: %v", err)
		return nil, nil, nil
	}
	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	return writer, encoder, file
}

// runnerのcontextを準備してイベント消費ループを起動する。
func (r *runner) run(parent context.Context) {
	r.ctx, r.cancel = context.WithCancel(parent)
	go r.consume()
}

// upstreamイベントとタイマーイベントを受け取り続ける。
func (r *runner) consume() {
	defer close(r.downstream)
	for {
		select {
		case <-r.ctx.Done():
			r.stopTimer()
			return
		case evt, ok := <-r.upstream:
			if !ok {
				r.stopTimer()
				return
			}
			r.handleEvent(evt)
		case <-r.timerC:
			r.timerC = nil
			r.timer = nil
			r.applyEffects(r.core.Handle(timerElapsedSignal{}))
		}
	}
}

// 外部イベントを内部signalへ変換してcoreへ渡す。
func (r *runner) handleEvent(evt types.Event) {
	sig, ok := signalFromEvent(evt)
	if !ok {
		return
	}
	r.applyEffects(r.core.Handle(sig))
}

// runnerを停止して入力チャネルとloggerを閉じる。
func (r *runner) close() error {
	if r.once {
		return nil
	}
	r.once = true
	if r.cancel != nil {
		r.cancel()
	}
	if err := r.logger.Close(); err != nil {
		log.Printf("conversation: log close error: %v", err)
	}
	close(r.upstream)
	return nil
}

// ここから下は、coreが返したeffectをruntime処理へ割り当てる領域です。

// effectの種類ごとにruntime処理を実行する。
func (r *runner) applyEffects(effects []effect) {
	for _, eff := range effects {
		switch e := eff.(type) {
		case emitEventEffect:
			r.emit(e.event)
		case startTimerEffect:
			r.startTimer(e.duration)
		case stopTimerEffect:
			r.stopTimer()
		case requestResponseEffect:
			r.applyRequestResponseEffect(e)
		case logRecordEffect:
			r.applyLogRecordEffect(e)
		case runtimeLogEffect:
			r.applyRuntimeLogEffect(e)
		}
	}
}

// 会話スナップショット更新イベントを作る。
func emitConversationSnapshotEffect(messages []types.ChatMessage) emitEventEffect {
	cloned := make([]types.ChatMessage, len(messages))
	copy(cloned, messages)
	return emitEventEffect{
		event: types.Event{
			Kind: types.EventConversationSnapshotUpdated,
			Payload: types.ConversationSnapshot{
				Messages: cloned,
			},
		},
	}
}

// 会話アクティビティ通知イベントを作る。
func emitConversationActivityEffect(at time.Time, source string) emitEventEffect {
	return emitEventEffect{
		event: types.Event{
			Kind: types.EventConversationActivity,
			Payload: types.ConversationActivity{
				At:     at,
				Source: source,
			},
		},
	}
}

// downstreamへイベントを送る。
func (r *runner) emit(evt types.Event) {
	select {
	case <-r.ctx.Done():
		return
	case r.downstream <- evt:
	}
}

// 会話ログeffectをloggerへ書き込む。
func (r *runner) applyLogRecordEffect(e logRecordEffect) {
	r.logger.Write(e.record)
}

// runtimeログeffectを標準loggerへ出力する。
func (r *runner) applyRuntimeLogEffect(e runtimeLogEffect) {
	if e.message != "" {
		log.Print(e.message)
	}
}

// AI応答リクエストeffectを外部イベントとして送る。
func (r *runner) applyRequestResponseEffect(e requestResponseEffect) {
	messages := r.contexts.WithSystemContexts(r.ctx, e.messages)
	if len(messages) == 0 {
		return
	}
	r.emit(types.Event{
		Kind: types.EventResponsesRequest,
		Payload: types.ResponsesRequest{
			RequestID: e.requestID,
			Messages:  messages,
			Tools:     e.tools,
		},
	})
}

// 指定時間後にtimerElapsedSignalを発火するタイマーを開始する。
func (r *runner) startTimer(d time.Duration) {
	if d <= 0 {
		r.stopTimer()
		r.applyEffects(r.core.Handle(timerElapsedSignal{}))
		return
	}
	if r.timer == nil {
		r.timer = time.NewTimer(d)
		r.timerC = r.timer.C
		return
	}
	if !r.timer.Stop() {
		select {
		case <-r.timer.C:
		default:
		}
	}
	r.timer.Reset(d)
	r.timerC = r.timer.C
}

// 実行中のタイマーを停止して参照を消す。
func (r *runner) stopTimer() {
	if r.timer == nil {
		return
	}
	if !r.timer.Stop() {
		select {
		case <-r.timer.C:
		default:
		}
	}
	r.timer = nil
	r.timerC = nil
}

// coreが返したeffectをruntime処理へ割り当てる領域はここまでです。
