package conversationstarter

import (
	"context"
	"strings"
	"time"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

// Config defines how ConversationStarter triggers proactive prompts.
type Config struct {
	Interval time.Duration
	Prompt   string
}

// Stage emits system EventTextInput at configured intervals.
type Stage struct {
	cfg        Config
	upstream   chan types.Event
	downstream chan types.Event
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewStage(ctx context.Context, cfg Config) *Stage {
	cctx, cancel := context.WithCancel(ctx)
	s := &Stage{
		cfg:        cfg,
		upstream:   make(chan types.Event),
		downstream: make(chan types.Event),
		ctx:        cctx,
		cancel:     cancel,
	}
	go s.drainUpstream()
	go s.produce()
	return s
}

func (s *Stage) drainUpstream() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case _, ok := <-s.upstream:
			if !ok {
				return
			}
		}
	}
}

func (s *Stage) produce() {
	defer close(s.downstream)
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			text := strings.TrimSpace(s.cfg.Prompt)
			if text == "" {
				continue
			}
			evt := types.Event{
				Kind: types.EventTextInput,
				Payload: types.OutputLine{
					Role: "system",
					Text: text,
				},
			}
			select {
			case <-s.ctx.Done():
				return
			case s.downstream <- evt:
			}
		}
	}
}

func (s *Stage) Upstream() chan<- types.Event { return s.upstream }

func (s *Stage) Downstream() <-chan types.Event { return s.downstream }

func (s *Stage) Close() error {
	s.cancel()
	close(s.upstream)
	return nil
}

var _ graph.Stage = (*Stage)(nil)
