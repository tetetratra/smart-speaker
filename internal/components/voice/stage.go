package voice

import (
	"context"
	"errors"
	"io"
	"log"

	"smart-speaker/internal/components/filereader"
	"smart-speaker/internal/components/micreader"
	"smart-speaker/internal/graph"
	"smart-speaker/internal/interfaces"
	types "smart-speaker/internal/types"
)

// Stage streams audio chunks from microphone or file.
type Stage struct {
	reader interfaces.Reader[types.AudioChunk]
}

// NewStage creates a voice stage based on INPUT_VOICE path.
func NewStage(inputPath string) (*Stage, error) {
	if inputPath != "" {
		reader, err := filereader.New(inputPath)
		if err != nil {
			return nil, err
		}
		return &Stage{reader: reader}, nil
	}
	reader, err := micreader.New()
	if err != nil {
		return nil, err
	}
	return &Stage{reader: reader}, nil
}

// Process ignores upstream and produces audio chunks.
func (s *Stage) Process(ctx context.Context, upstream <-chan interface{}) <-chan interface{} {
	_ = upstream
	out := make(chan interface{})
	go func() {
		defer close(out)
		for {
			chunk, err := s.reader.Read(ctx)
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.Canceled) {
					return
				}
				log.Printf("voice stage read error: %v", err)
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

// Close releases the underlying reader.
func (s *Stage) Close() error {
	return s.reader.Close()
}

var _ graph.Stage = (*Stage)(nil)
