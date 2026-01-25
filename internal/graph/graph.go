package graph

import (
	"context"
	"errors"
	"log"
	"sync"

	types "smart-speaker/internal/types"
)

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

	eventDetailFormatters map[types.EventKind]EventDetailFormatter
}

func New() *Graph {
	return &Graph{eventDetailFormatters: defaultEventDetailFormatters()}
}

func (g *Graph) AddNode(stage *Stage) *Node {
	n := &Node{Stage: stage}
	g.nodes = append(g.nodes, n)
	return n
}

func (g *Graph) Connect(from, to *Node) {
	if from.Stage.Downstream == nil {
		panic("graph: from stage must have downstream")
	}
	if to.Stage.Upstream == nil {
		panic("graph: to stage must have upstream")
	}
	g.edges = append(g.edges, &Edge{From: from, To: to})
}

// Run は各エッジごとに goroutine を起動し、Stage 間のチャネル転送を行う。
func (g *Graph) Run(ctx context.Context) error {
	log.Printf("graph nodes=%d edges=%d", len(g.nodes), len(g.edges))
	adj := make(map[*Node][]*Stage, len(g.nodes))
	for _, edge := range g.edges {
		adj[edge.From] = append(adj[edge.From], edge.To.Stage)
	}

	for _, node := range g.nodes {
		go node.Stage.Run(ctx)
	}

	var wg sync.WaitGroup
	for _, node := range g.nodes {
		downstreams := adj[node]
		out := (<-chan types.Event)(node.Stage.Downstream)
		wg.Add(1)
		go func(out <-chan types.Event, downstreams []*Stage) {
			log.Printf("graph pump start from %p to %d downstreams", out, len(downstreams))
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case val, ok := <-out:
					if !ok {
						return
					}
					if len(downstreams) > 0 {
						log.Printf("%s", g.formatForwardLog(node.Stage, downstreams, val))
					}
					for _, dst := range downstreams {
						in := (chan<- types.Event)(dst.Upstream)
						in <- val
					}
				}
			}
		}(out, downstreams)
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
		node := g.nodes[i]
		if err := node.Stage.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
