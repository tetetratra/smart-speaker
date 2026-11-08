package graph

import (
	"context"
	"errors"
	"log"
	"sync"

	"github.com/tetetratra/smart-speaker/internal/eventqueue"
	types "github.com/tetetratra/smart-speaker/internal/types"
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

// Run は宛先ごとの無制限キューへ投入し、後段の空き待ちを送信元から切り離す。
func (g *Graph) Run(ctx context.Context) error {
	log.Printf("graph nodes=%d edges=%d", len(g.nodes), len(g.edges))
	adj := make(map[*Node][]edgeTarget, len(g.nodes))
	for _, edge := range g.edges {
		adj[edge.From] = append(adj[edge.From], edgeTarget{stage: edge.To.Stage, kinds: edge.Kinds})
	}

	var wg sync.WaitGroup
	queues := make(map[*Stage]*eventqueue.Queue[types.Event], len(g.nodes))
	for _, node := range g.nodes {
		st := node.Stage
		var generation func(types.Event) (uint64, bool)
		if st.LatestOnly {
			generation = types.EventGeneration
		}
		q := eventqueue.New(generation)
		queues[st] = q
		go st.Run(ctx)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				e, ok := q.Pop(ctx)
				if !ok {
					return
				}
				select {
				case <-ctx.Done():
					return
				case st.Upstream <- e:
				}
			}
		}()
	}
	for _, node := range g.nodes {
		targets := adj[node]
		out := node.Stage.Downstream
		wg.Add(1)
		go func() {
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
					if g.shouldLogForwardEvent(val.Kind) && len(downstreams) > 0 {
						log.Printf("%s", g.formatForwardLog(node.Stage, downstreams, val))
					}
					for _, dst := range downstreams {
						queues[dst].Push(val)
					}
				}
			}
		}()
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
