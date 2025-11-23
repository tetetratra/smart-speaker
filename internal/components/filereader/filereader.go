package filereader

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

type fileReader struct {
	source          *FileSource
	downstream      chan types.Event
	ctx             context.Context
	cancel          context.CancelFunc
	once            sync.Once
	closerWaitGroup sync.WaitGroup
}

// NewStage wires the file reader into the graph.Stage contract.
func NewStage(path string) (*graph.Stage, error) {
	source, err := NewFileSource(path)
	if err != nil {
		return nil, err
	}
	s := &fileReader{
		source:     source,
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
	}
	return &graph.Stage{
		Upstream:   nil,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.close,
	}, nil
}

func (s *fileReader) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.ctx = ctx
	s.cancel = cancel
	s.closerWaitGroup.Add(1)
	go func() {
		defer s.closerWaitGroup.Done()
		s.produce()
	}()
}

func (s *fileReader) produce() {
	defer close(s.downstream)
	for {
		chunk, err := s.source.Read(s.ctx)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("filereader stage read error: %v", err)
			return
		}
		evt := types.Event{Kind: types.EventAudioChunk, Payload: chunk}
		select {
		case <-s.ctx.Done():
			return
		case s.downstream <- evt:
		}
	}
}

func (s *fileReader) close() error {
	var err error
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.closerWaitGroup.Wait()
		err = s.source.Close()
		log.Println("filereader: stage closed")
	})
	return err
}
