package graph

import (
	"context"
	"errors"
	"sync"

	types "smart-speaker/internal/types"
)

// Stage はグラフに接続される処理ノードのチャネルとライフサイクルフック。
type Stage struct {
	Upstream   chan types.Event
	Downstream chan types.Event
	Run        func(context.Context)
	CloseFn    func() error
	closeOnce  sync.Once
}

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

type Node struct {
	Stage *Stage
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

func (g *Graph) AddNode(stage *Stage) *Node {
	if stage == nil {
		return nil
	}
	n := &Node{Stage: stage}
	g.nodes = append(g.nodes, n)
	return n
}

func (g *Graph) Connect(from, to *Node) {
	g.edges = append(g.edges, &Edge{From: from, To: to})
}

// Run は各エッジごとに goroutine を起動し、Stage 間のチャネル転送を行う。
func (g *Graph) Run(ctx context.Context) error {
	adj := make(map[*Node][]*Stage, len(g.nodes))
	for _, edge := range g.edges {
		if edge.From == nil || edge.To == nil {
			continue
		}
		adj[edge.From] = append(adj[edge.From], edge.To.Stage)
	}

	var stageWG sync.WaitGroup
	for _, node := range g.nodes {
		if node == nil || node.Stage == nil || node.Stage.Run == nil {
			continue
		}
		stageWG.Add(1)
		go func(st *Stage) {
			defer stageWG.Done()
			st.Run(ctx)
		}(node.Stage)
	}

	var wg sync.WaitGroup
	for _, node := range g.nodes {
		if node == nil {
			continue
		}
		downstreams := adj[node]
		if len(downstreams) == 0 {
			continue
		}
		out := (<-chan types.Event)(node.Stage.Downstream)
		if out == nil {
			continue
		}
		wg.Add(1)
		go func(out <-chan types.Event, downstreams []*Stage) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case val, ok := <-out:
					if !ok {
						return
					}
					for _, dst := range downstreams {
						if dst == nil || dst.Upstream == nil {
							continue
						}
						in := (chan<- types.Event)(dst.Upstream)
						select {
						case <-ctx.Done():
							return
						case in <- val:
						}
					}
				}
			}
		}(out, downstreams)
	}

	wg.Wait()
	stageWG.Wait()
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func (g *Graph) Close() error {
	var firstErr error
	for i := len(g.nodes) - 1; i >= 0; i-- {
		node := g.nodes[i]
		if node == nil || node.Stage == nil {
			continue
		}
		if err := node.Stage.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
