package graph

import (
	"context"
)

// Stage represents a processing node within the graph.
type Stage interface {
	Process(ctx context.Context, upstream <-chan interface{}) <-chan interface{}
	Close() error
}

// Graph executes stages sequentially.
type Graph struct {
	stages []Stage
}

// New creates an empty graph.
func New() *Graph {
	return &Graph{}
}

// Add registers a stage to the execution list.
func (g *Graph) Add(stage Stage) {
	g.stages = append(g.stages, stage)
}

// Run wires stages sequentially and starts execution.
func (g *Graph) Run(ctx context.Context) error {
	upstream := (<-chan interface{})(nil)
	for _, stage := range g.stages {
		upstream = stage.Process(ctx, upstream)
	}
	<-ctx.Done()
	return nil
}

// Close releases stages in reverse order.
func (g *Graph) Close() error {
	var firstErr error
	for i := len(g.stages) - 1; i >= 0; i-- {
		if err := g.stages[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
