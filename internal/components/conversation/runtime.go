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

func newConversationLogger(path string) *conversationLogger {
	writer, encoder, file := openLogWriter(path)
	return &conversationLogger{
		file:    file,
		writer:  writer,
		encoder: encoder,
	}
}

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

func (r *runner) run(parent context.Context) {
	r.ctx, r.cancel = context.WithCancel(parent)
	go r.consume()
}

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

func (r *runner) handleEvent(evt types.Event) {
	sig, ok := signalFromEvent(evt)
	if !ok {
		return
	}
	r.applyEffects(r.core.Handle(sig))
}

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

func (r *runner) emit(evt types.Event) {
	select {
	case <-r.ctx.Done():
		return
	case r.downstream <- evt:
	}
}

func (r *runner) applyLogRecordEffect(e logRecordEffect) {
	r.logger.Write(e.record)
}

func (r *runner) applyRuntimeLogEffect(e runtimeLogEffect) {
	if e.message != "" {
		log.Print(e.message)
	}
}

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
