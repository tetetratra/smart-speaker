package micreader

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

type micStage struct {
	source          *MicSource
	downstream      chan types.Event
	ctx             context.Context
	cancel          context.CancelFunc
	once            sync.Once
	closerWaitGroup sync.WaitGroup
}

// NewStage exposes microphone reader as graph.Stage.
func NewStage() (*graph.Stage, error) {
	source, err := NewMicSource()
	if err != nil {
		return nil, err
	}
	s := &micStage{
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

func (s *micStage) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.ctx = ctx
	s.cancel = cancel
	log.Println("🎙 マイク入力を待機しています。CTRL+Cで終了します。")
	s.closerWaitGroup.Add(1)
	go func() {
		defer s.closerWaitGroup.Done()
		s.produce()
	}()
}

func (s *micStage) produce() {
	defer close(s.downstream)
	for {
		chunk, err := s.source.Read(s.ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
				return
			}
			log.Printf("micreader stage read error: %v", err)
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

func (s *micStage) close() error {
	var err error
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.closerWaitGroup.Wait()
		err = s.source.Close()
		log.Println("micreader: stage closed")
	})
	return err
}
