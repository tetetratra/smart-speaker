package graph

import (
	"context"
	"errors"
	"log"
	"reflect"
	"sync"

	"github.com/tetetratra/smart-speaker/internal/eventqueue"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type Node struct {
	Stage *Stage
}

type Edge struct {
	From *Node
	To   *Node
	// Types は、この辺で流すイベントの型。空ならすべてを流す。
	Types map[reflect.Type]struct{}
}

type Graph struct {
	nodes []*Node
	edges []*Edge

	suppressedForwardLogTypes map[reflect.Type]struct{}
}

func New() *Graph {
	return &Graph{suppressedForwardLogTypes: defaultSuppressedForwardLogTypes()}
}

func (g *Graph) AddNode(stage *Stage) *Node {
	n := &Node{Stage: stage}
	g.nodes = append(g.nodes, n)
	return n
}

// Connect は from から to へ、events と同じ型のイベントだけを流す。events にはゼロ値を渡す（例: types.EventToolRequest{}）。
func (g *Graph) Connect(from, to *Node, events ...types.Event) {
	allowed := make(map[reflect.Type]struct{}, len(events))
	for _, event := range events {
		allowed[reflect.TypeOf(event)] = struct{}{}
	}
	g.connect(from, to, allowed)
}

func (g *Graph) connect(from, to *Node, eventTypes map[reflect.Type]struct{}) {
	if from.Stage.Downstream == nil {
		panic("graph: from stage must have downstream")
	}
	if to.Stage.Upstream == nil {
		panic("graph: to stage must have upstream")
	}
	g.edges = append(g.edges, &Edge{From: from, To: to, Types: eventTypes})
}

// Run は宛先ごとの無制限キューへ投入し、後段の空き待ちを送信元から切り離す。
func (g *Graph) Run(ctx context.Context) error {
	log.Printf("graph nodes=%d edges=%d", len(g.nodes), len(g.edges))
	adj := make(map[*Node][]edgeTarget, len(g.nodes))
	for _, edge := range g.edges {
		adj[edge.From] = append(adj[edge.From], edgeTarget{stage: edge.To.Stage, types: edge.Types})
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
					eventType := reflect.TypeOf(val)
					downstreams := matchingStages(targets, eventType)
					if g.shouldLogForwardEvent(eventType) && len(downstreams) > 0 {
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
	types map[reflect.Type]struct{}
}

func matchingStages(targets []edgeTarget, eventType reflect.Type) []*Stage {
	stages := make([]*Stage, 0, len(targets))
	for _, target := range targets {
		if len(target.types) > 0 {
			if _, ok := target.types[eventType]; !ok {
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
