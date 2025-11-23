package realtimeapi

import (
	"context"
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
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		ctx:        stageCtx,
		cancel:     cancel,
		voice:      cfg.Voice,
	}
	return &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.close,
	}, nil
}

func (s *realtimeAPI) run(context.Context) {
	s.closerWaitGroup.Add(2)
	senderRunner := sender.NewRunner(s.ctx, s.client, s.upstream, s.voice)
	receiverRunner := receiver.NewRunner(s.ctx, s.stream, s.downstream)
	go func() {
		defer s.closerWaitGroup.Done()
		senderRunner.Run()
	}()
	go func() {
		defer s.closerWaitGroup.Done()
		receiverRunner.Run()
	}()
}

// Close closes the underlying client and owned channels.
func (s *realtimeAPI) close() error {
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
