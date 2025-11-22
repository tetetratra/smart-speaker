package realtimeapi

import (
	"context"
	"errors"
	"log"
	"sync"

	"smart-speaker/internal/components/realtimeapi/receiver"
	"smart-speaker/internal/components/realtimeapi/sender"
	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

type realtimeAPI struct {
	client          *Client
	stream          *receiver.EventStream
	upstream        chan types.Event
	downstream      chan types.Event
	ctx             context.Context
	cancel          context.CancelFunc
	once            sync.Once
	closerWaitGroup sync.WaitGroup
	voice           string
}

// NewStage constructs a realtime stage with the given config.
func NewStage(ctx context.Context, cfg Config) (*graph.Stage, error) {
	stageCtx, cancel := context.WithCancel(ctx)
	client, err := NewClient(stageCtx, cfg)
	if err != nil {
		cancel()
		return nil, err
	}
	s := &realtimeAPI{
		client:     client,
		stream:     receiver.NewEventStream(client),
		upstream:   make(chan types.Event),
		downstream: make(chan types.Event),
		ctx:        stageCtx,
		cancel:     cancel,
		voice:      cfg.Voice,
	}
	return &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.Close,
	}, nil
}

func (s *realtimeAPI) run(context.Context) {
	s.closerWaitGroup.Add(2)
	senderRunner := sender.NewRunner(s.ctx, s.client, s.upstream, s.voice)
	go func() {
		defer s.closerWaitGroup.Done()
		senderRunner.Run()
	}()
	go func() {
		defer s.closerWaitGroup.Done()
		s.runReceiver()
	}()
}

func (s *realtimeAPI) runReceiver() {
	defer close(s.downstream)
	for {
		evt, err := s.stream.Next(s.ctx)
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
		case s.downstream <- evt:
		}
	}
}

// Close closes the underlying client and owned channels.
func (s *realtimeAPI) Close() error {
	var err error
	s.once.Do(func() {
		s.cancel()
		close(s.upstream)
		s.closerWaitGroup.Wait()
		err = s.client.Close()
		log.Println("realtimeapi: stage closed")
	})
	return err
}
