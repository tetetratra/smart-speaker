package graph

import (
	"context"
	"sync"

	types "smart-speaker/internal/types"
)

const DefaultChannelBufferSize = 64

// Stage represents an executable node in the graph with upstream/downstream channels.
type Stage struct {
	Upstream   chan types.Event
	Downstream chan types.Event
	Run        func(context.Context)
	CloseFn    func() error
	closeOnce  sync.Once
}

// Close invokes the stage's close handler once.
func (s *Stage) Close() error {
	if s == nil {
		return nil
	}
	var err error
	s.closeOnce.Do(func() {
		if s.CloseFn != nil {
			err = s.CloseFn()
		} else if s.Upstream != nil {
			close(s.Upstream)
		}
	})
	return err
}
