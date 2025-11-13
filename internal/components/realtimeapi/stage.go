package realtimeapi

import (
	"context"
	"errors"
	"log"
	"sync"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

// Stage wraps the Realtime API client into the new graph.Stage interface.
type Stage struct {
	client     *Client
	upstream   chan interface{}
	downstream chan interface{}
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once
}

// NewStage constructs a realtime stage with the given config.
func NewStage(ctx context.Context, cfg Config) (*Stage, error) {
	stageCtx, cancel := context.WithCancel(ctx)
	client, err := NewClient(stageCtx, cfg)
	if err != nil {
		cancel()
		return nil, err
	}
	s := &Stage{
		client:     client,
		upstream:   make(chan interface{}),
		downstream: make(chan interface{}),
		ctx:        stageCtx,
		cancel:     cancel,
	}
	go s.runSender()
	go s.runReceiver()
	return s, nil
}

func (s *Stage) runSender() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case data, ok := <-s.upstream:
			if !ok {
				return
			}
			chunk, ok := data.(types.AudioChunk)
			if !ok {
				log.Printf("unexpected upstream data type: %T", data)
				continue
			}
			if err := s.client.Process(s.ctx, chunk); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				log.Printf("realtime send error: %v", err)
				return
			}
		}
	}
}

func (s *Stage) runReceiver() {
	defer close(s.downstream)
	for {
		line, err := s.client.Read(s.ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("realtime read error: %v", err)
			return
		}
		select {
		case <-s.ctx.Done():
			return
		case s.downstream <- line:
		}
	}
}

func (s *Stage) Upstream() chan<- interface{} { return s.upstream }

func (s *Stage) Downstream() <-chan interface{} { return s.downstream }

// Close closes the underlying client and owned channels.
func (s *Stage) Close() error {
	var err error
	s.once.Do(func() {
		s.cancel()
		close(s.upstream)
		err = s.client.Close()
	})
	return err
}

var _ graph.Stage = (*Stage)(nil)
