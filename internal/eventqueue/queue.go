package eventqueue

import (
	"context"
	"sync"
)

// Queue は後段の空き待ちに投入側を巻き込まない、容量無制限の待機列。
// 世代を持たないイベントは順序を維持し、世代を持つイベントは最新世代だけを残す。
type Queue[T any] struct {
	mu         sync.Mutex
	items      []T
	latest     uint64
	generation func(T) (uint64, bool)
	wake       chan struct{}
}

func New[T any](generation func(T) (uint64, bool)) *Queue[T] {
	return &Queue[T]{generation: generation, wake: make(chan struct{}, 1)}
}

func (q *Queue[T]) Push(v T) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.generation != nil {
		if id, ok := q.generation(v); ok {
			if id < q.latest {
				return
			}
			if id > q.latest {
				q.latest = id
				kept := q.items[:0]
				for _, old := range q.items {
					if oldID, has := q.generation(old); !has || oldID >= id {
						kept = append(kept, old)
					}
				}
				var zero T
				for i := len(kept); i < len(q.items); i++ {
					q.items[i] = zero
				}
				q.items = kept
			}
		}
	}
	q.items = append(q.items, v)
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *Queue[T]) Pop(ctx context.Context) (T, bool) {
	for {
		if ctx.Err() != nil {
			var zero T
			return zero, false
		}
		q.mu.Lock()
		if len(q.items) > 0 {
			v := q.items[0]
			var zero T
			q.items[0] = zero
			q.items = q.items[1:]
			if len(q.items) == 0 {
				q.items = nil
			}
			q.mu.Unlock()
			return v, true
		}
		q.mu.Unlock()
		select {
		case <-ctx.Done():
			var zero T
			return zero, false
		case <-q.wake:
		}
	}
}
