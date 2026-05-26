package stt

import (
	"context"
	"io"
	"testing"
	"time"

	speechpb "cloud.google.com/go/speech/apiv2/speechpb"
	types "github.com/tetetratra/smart-speaker/internal/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

func TestRecognizerPath(t *testing.T) {
	got := recognizerPath(" project-a ", " recognizer-a ")
	want := "projects/project-a/locations/asia-northeast1/recognizers/recognizer-a"
	if got != want {
		t.Fatalf("recognizerPath() = %q, want %q", got, want)
	}

	got = recognizerPath("project-a", " ")
	want = "projects/project-a/locations/asia-northeast1/recognizers/_"
	if got != want {
		t.Fatalf("recognizerPath() = %q, want %q", got, want)
	}
}

func TestSendSpeechAudioChunksEvenByteBoundaries(t *testing.T) {
	stream := &fakeSpeechStream{}
	s := &stage{speechStream: stream}
	audio := make([]byte, speechAudioChunkBytes+3)
	for i := range audio {
		audio[i] = byte(i % 251)
	}

	s.sendSpeechAudio(audio)

	if len(stream.sent) != 2 {
		t.Fatalf("expected 2 audio chunks, got %d", len(stream.sent))
	}
	if got := len(stream.sent[0].GetAudio()); got != speechAudioChunkBytes {
		t.Fatalf("first chunk length = %d, want %d", got, speechAudioChunkBytes)
	}
	if got := len(stream.sent[1].GetAudio()); got != 2 {
		t.Fatalf("second chunk length = %d, want 2", got)
	}
}

func TestHandleSpeechAudioEndSchedulesCloseSend(t *testing.T) {
	stream := &fakeSpeechStream{}
	s := &stage{speechStream: stream}

	s.handleSpeechAudio(types.RTCSpeechAudio{Type: types.RTCSpeechAudioEnd})

	if !waitUntil(func() bool { return stream.closed }) {
		t.Fatal("expected speech stream to be closed")
	}
	if s.speechStream != nil {
		t.Fatal("expected stage speech stream to be cleared")
	}
}

type fakeSpeechStream struct {
	sent   []*speechpb.StreamingRecognizeRequest
	closed bool
}

func (f *fakeSpeechStream) Send(req *speechpb.StreamingRecognizeRequest) error {
	f.sent = append(f.sent, req)
	return nil
}

func (f *fakeSpeechStream) Recv() (*speechpb.StreamingRecognizeResponse, error) {
	return nil, io.EOF
}

func (f *fakeSpeechStream) Header() (metadata.MD, error) {
	return nil, nil
}

func (f *fakeSpeechStream) Trailer() metadata.MD {
	return nil
}

func (f *fakeSpeechStream) CloseSend() error {
	f.closed = true
	return nil
}

func (f *fakeSpeechStream) Context() context.Context {
	return context.Background()
}

func (f *fakeSpeechStream) SendMsg(m any) error {
	return nil
}

func (f *fakeSpeechStream) RecvMsg(m any) error {
	return io.EOF
}

func waitUntil(ok func() bool) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return ok()
}
