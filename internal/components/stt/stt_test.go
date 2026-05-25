package stt

import (
	"context"
	"io"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsExpectedSpeechStreamClose(t *testing.T) {
	t.Run("context canceled", func(t *testing.T) {
		if !isExpectedSpeechStreamClose(context.Canceled) {
			t.Fatal("context.Canceled should be expected")
		}
	})

	t.Run("io EOF", func(t *testing.T) {
		if !isExpectedSpeechStreamClose(io.EOF) {
			t.Fatal("io.EOF should be expected")
		}
	})

	t.Run("grpc canceled", func(t *testing.T) {
		err := status.Error(codes.Canceled, "context canceled")
		if !isExpectedSpeechStreamClose(err) {
			t.Fatal("grpc codes.Canceled should be expected")
		}
	})

	t.Run("other grpc error", func(t *testing.T) {
		err := status.Error(codes.Unavailable, "temporary unavailable")
		if isExpectedSpeechStreamClose(err) {
			t.Fatal("codes.Unavailable should not be expected")
		}
	})
}

func TestBuildSpeechAdaptation(t *testing.T) {
	adaptation := buildSpeechAdaptation([]string{" your-username ", "", "スマートスピーカー"})
	if adaptation == nil {
		t.Fatal("expected adaptation")
	}
	if len(adaptation.PhraseSets) != 1 {
		t.Fatalf("expected 1 phrase set, got %d", len(adaptation.PhraseSets))
	}
	phraseSet := adaptation.PhraseSets[0].GetInlinePhraseSet()
	if phraseSet == nil {
		t.Fatal("expected inline phrase set")
	}
	if phraseSet.Boost != 20 {
		t.Fatalf("expected boost 20, got %v", phraseSet.Boost)
	}
	got := phraseSet.Phrases
	if len(got) != 2 {
		t.Fatalf("expected 2 phrases, got %d", len(got))
	}
	if got[0].Value != "your-username" || got[1].Value != "スマートスピーカー" {
		t.Fatalf("unexpected phrases: %#v", got)
	}
}

func TestBuildSpeechAdaptationReturnsNilWithoutPhrases(t *testing.T) {
	if got := buildSpeechAdaptation([]string{" ", ""}); got != nil {
		t.Fatalf("expected nil adaptation, got %#v", got)
	}
}
