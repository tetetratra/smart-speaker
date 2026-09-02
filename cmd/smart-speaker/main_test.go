package main

import (
	"testing"

	"github.com/tetetratra/smart-speaker/internal/app"
)

func TestBuildSTTStageDefaultsToGoogle(t *testing.T) {
	st, err := buildSTTStage(app.Config{
		APIKey: "openai",
	})
	if err != nil {
		t.Fatal(err)
	}
	if st == nil {
		t.Fatal("expected stage")
	}
	_ = st.Close()
}

func TestBuildSTTStageSelectsOpenAI(t *testing.T) {
	st, err := buildSTTStage(app.Config{
		APIKey:         "openai",
		STTProvider:    "openai",
		OpenAISTTModel: "gpt-realtime-whisper",
	})
	if err != nil {
		t.Fatal(err)
	}
	if st == nil {
		t.Fatal("expected stage")
	}
	_ = st.Close()
}

func TestBuildSTTStageRejectsUnknownProvider(t *testing.T) {
	if _, err := buildSTTStage(app.Config{STTProvider: "unknown"}); err == nil {
		t.Fatal("expected error")
	}
}
