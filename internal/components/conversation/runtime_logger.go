package conversation

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type logRecord struct {
	Timestamp       string   `json:"ts"`
	Speaker         string   `json:"speaker"`
	Text            string   `json:"text"`
	ResponseID      string   `json:"response_id,omitempty"`
	Source          string   `json:"source,omitempty"`
	ReactionLevel   string   `json:"reaction_level,omitempty"`
	ReactionReasons []string `json:"reaction_reasons,omitempty"`
	ReactionScore   *int     `json:"reaction_score,omitempty"`
	PassedToLLM     *bool    `json:"passed_to_llm,omitempty"`
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
