package graph

import (
	"context"
	"sync"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

const DefaultChannelBufferSize = 64

// Stage represents an executable node in the graph with upstream/downstream channels.
type Stage struct {
	Name       string
	Upstream   chan types.Event
	Downstream chan types.Event
	Run        func(context.Context)
	CloseFn    func() error
	// LatestOnly は応答出力キューで古い世代を捨てる。
	LatestOnly bool
	// ReceiveFilter は最後の受け渡し後、処理開始前に接続部分で適用する。
	ReceiveFilter func(context.Context, types.Event) bool
	closeOnce     sync.Once
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

// Receive は受信待ちの間に変わった世代も、処理へ渡す前に確認する。
func (s *Stage) Receive(ctx context.Context) (types.Event, bool) {
	for {
		select {
		case <-ctx.Done():
			return types.Event{}, false
		case e, ok := <-s.Upstream:
			if !ok {
				return types.Event{}, false
			}
			if s.ReceiveFilter == nil || s.ReceiveFilter(ctx, e) {
				return e, true
			}
		}
	}
}
