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
	From  *Node
	To    *Node
	Kinds map[types.EventKind]struct{}
}

type Graph struct {
	nodes []*Node
	edges []*Edge

	eventDetailFormatters     map[types.EventKind]EventDetailFormatter
	suppressedForwardLogKinds map[types.EventKind]struct{}
}

func New() *Graph {
	return &Graph{
		eventDetailFormatters:     defaultEventDetailFormatters(),
		suppressedForwardLogKinds: defaultSuppressedForwardLogKinds(),
	}
}

func (g *Graph) AddNode(stage *Stage) *Node {
	n := &Node{Stage: stage}
	g.nodes = append(g.nodes, n)
	return n
}

func (g *Graph) Connect(from, to *Node) {
	g.connect(from, to, nil)
}

func (g *Graph) ConnectKinds(from, to *Node, kinds ...types.EventKind) {
	allowed := make(map[types.EventKind]struct{}, len(kinds))
	for _, kind := range kinds {
		allowed[kind] = struct{}{}
	}
	g.connect(from, to, allowed)
}

func (g *Graph) connect(from, to *Node, kinds map[types.EventKind]struct{}) {
	if from.Stage.Downstream == nil {
		panic("graph: from stage must have downstream")
	}
	if to.Stage.Upstream == nil {
		panic("graph: to stage must have upstream")
	}
	g.edges = append(g.edges, &Edge{From: from, To: to, Kinds: kinds})
}

// Run は各エッジごとに goroutine を起動し、Stage 間のチャネル転送を行う。
func (g *Graph) Run(ctx context.Context) error {
	log.Printf("graph nodes=%d edges=%d", len(g.nodes), len(g.edges))
	adj := make(map[*Node][]edgeTarget, len(g.nodes))
	for _, edge := range g.edges {
		adj[edge.From] = append(adj[edge.From], edgeTarget{stage: edge.To.Stage, kinds: edge.Kinds})
	}

	for _, node := range g.nodes {
		go node.Stage.Run(ctx)
	}

	var wg sync.WaitGroup
	for _, node := range g.nodes {
		targets := adj[node]
		out := (<-chan types.Event)(node.Stage.Downstream)
		wg.Add(1)
		go func(out <-chan types.Event, targets []edgeTarget) {
			log.Printf("graph pump start from %p to %d downstreams", out, len(targets))
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case val, ok := <-out:
					if !ok {
						return
					}
					downstreams := matchingStages(targets, val.Kind)
					if len(downstreams) == 0 {
						continue
					}
					if g.shouldLogForwardEvent(val.Kind) {
						log.Printf("%s", g.formatForwardLog(node.Stage, downstreams, val))
					}
					for _, dst := range downstreams {
						in := (chan<- types.Event)(dst.Upstream)
						in <- val
					}
				}
			}
		}(out, targets)
	}

	wg.Wait()
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

type edgeTarget struct {
	stage *Stage
	kinds map[types.EventKind]struct{}
}

func matchingStages(targets []edgeTarget, kind types.EventKind) []*Stage {
	stages := make([]*Stage, 0, len(targets))
	for _, target := range targets {
		if len(target.kinds) > 0 {
			if _, ok := target.kinds[kind]; !ok {
				continue
			}
		}
		stages = append(stages, target.stage)
	}
	return stages
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
