package conversation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConversationLoggerWritesReactionFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.jsonl")
	logger := newConversationLogger(path)

	score := -5
	passed := false
	logger.Write(logRecord{
		Speaker:         "reaction_gate",
		Text:            "まじか",
		Source:          "server-stt",
		ReactionLevel:   string(reactionIgnore),
		ReactionReasons: []string{"short_exclamation"},
		ReactionScore:   &score,
		PassedToLLM:     &passed,
	})
	if err := logger.Close(); err != nil {
		t.Fatalf("logger close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal log: %v; data=%s", err, string(data))
	}
	if got["speaker"] != "reaction_gate" {
		t.Fatalf("speaker = %v, want reaction_gate", got["speaker"])
	}
	if got["reaction_level"] != string(reactionIgnore) {
		t.Fatalf("reaction_level = %v, want %s", got["reaction_level"], reactionIgnore)
	}
	if got["reaction_score"] != float64(score) {
		t.Fatalf("reaction_score = %v, want %d", got["reaction_score"], score)
	}
	if got["passed_to_llm"] != passed {
		t.Fatalf("passed_to_llm = %v, want %t", got["passed_to_llm"], passed)
	}
}

func TestConversationLoggerKeepsExistingHumanRecordShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversation.jsonl")
	logger := newConversationLogger(path)

	logger.Write(logRecord{
		Speaker: "human",
		Text:    "こんにちは",
	})
	if err := logger.Close(); err != nil {
		t.Fatalf("logger close: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal log: %v; data=%s", err, string(data))
	}
	if got["speaker"] != "human" || got["text"] != "こんにちは" {
		t.Fatalf("record = %+v, want human text record", got)
	}
	if _, ok := got["reaction_level"]; ok {
		t.Fatalf("unexpected reaction_level in existing record: %+v", got)
	}
}
