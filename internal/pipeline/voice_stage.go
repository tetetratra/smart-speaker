package pipeline

import (
	"context"
	"errors"
	"io"
	"log"

	"smart-speaker/internal/components/filereader"
	"smart-speaker/internal/components/micreader"
	"smart-speaker/internal/interfaces"
	types "smart-speaker/internal/types"
)

// VoiceStage produces audio chunks either from microphone or file.
type VoiceStage struct {
	reader interfaces.Reader[types.AudioChunk]
}

// NewVoiceStage constructs a voice stage based on INPUT_VOICE.
func NewVoiceStage(inputPath string) (*VoiceStage, error) {
	if inputPath != "" {
		reader, err := filereader.New(inputPath)
		if err != nil {
			return nil, err
		}
		return &VoiceStage{reader: reader}, nil
	}
	reader, err := micreader.New()
	if err != nil {
		return nil, err
	}
	return &VoiceStage{reader: reader}, nil
}

func (v *VoiceStage) Process(ctx context.Context, upstream <-chan interface{}) <-chan interface{} {
	out := make(chan interface{})
	go func() {
		defer close(out)
		for {
			chunk, err := v.reader.Read(ctx)
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					return
				}
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}
				log.Printf("voice reader error: %v", err)
				return
			}
			select {
			case <-ctx.Done():
				return
			case out <- chunk:
			}
		}
	}()
	return out
}

func (v *VoiceStage) Close() error {
	return v.reader.Close()
}
