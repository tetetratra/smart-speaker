package eventqueue

import (
	"context"
	"testing"
)

type item struct {
	gen uint64
	n   int
}

func TestQueueDropsOldIncludingLateResultsAndRetainsHighWater(t *testing.T) {
	q := New(func(v item) (uint64, bool) { return v.gen, true })
	for i := 0; i < 10000; i++ {
		q.Push(item{1, i})
	}
	q.Push(item{2, 0})
	q.Push(item{1, 10000})
	q.Push(item{2, 1})
	for i := 0; i < 2; i++ {
		v, ok := q.Pop(context.Background())
		if !ok || v != (item{2, i}) {
			t.Fatal(v, ok)
		}
	}
	q.Push(item{1, 9})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := q.Pop(ctx); ok {
		t.Fatal("古い結果を受け入れた")
	}
	if len(q.items) != 0 {
		t.Fatal(q.items)
	}
}
func TestQueueAcceptsWhileConsumerBlocked(t *testing.T) {
	q := New[int](nil)
	for i := 0; i < 100000; i++ {
		q.Push(i)
	}
	for i := 0; i < 100000; i++ {
		v, ok := q.Pop(context.Background())
		if !ok || v != i {
			t.Fatal(i, v)
		}
	}
}
