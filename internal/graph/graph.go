package graph

import (
	"context"
	"errors"
	"sync"
)

// Stage は自身でチャネルを生成・管理し、その参照をメソッド経由で
// 露出する。Go のインターフェースではフィールドを直接要求できないため、
// Upstream/Downstream の getter がチャネルを渡す役割を担う。
type Stage interface {
	Upstream() chan<- interface{}
	Downstream() <-chan interface{}
	Close() error
}

type Node struct {
	Stage Stage
}

type Edge struct {
	From *Node
	To   *Node
}

type Graph struct {
	nodes []*Node
	edges []*Edge
}

func New() *Graph { return &Graph{} }

func (g *Graph) AddNode(stage Stage) *Node {
	n := &Node{Stage: stage}
	g.nodes = append(g.nodes, n)
	return n
}

func (g *Graph) Connect(from, to *Node) {
	g.edges = append(g.edges, &Edge{From: from, To: to})
}

// Run は各エッジごとに goroutine を起動し、Stage 間のチャネル転送を行う。
func (g *Graph) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for _, edge := range g.edges {
		from := edge.From.Stage
		to := edge.To.Stage
		wg.Add(1)
		go func(from Stage, to Stage) {
			defer wg.Done()
			out := from.Downstream()
			in := to.Upstream()
			for {
				select {
				case <-ctx.Done():
					return
				case val, ok := <-out:
					if !ok {
						return
					}
					select {
					case <-ctx.Done():
						return
					case in <- val:
					}
				}
			}
		}(from, to)
	}

	wg.Wait()
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (g *Graph) Close() error {
	var firstErr error
	for i := len(g.nodes) - 1; i >= 0; i-- {
		if err := g.nodes[i].Stage.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
