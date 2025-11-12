package pipeline

import (
	"context"
	"fmt"
)

// Stage represents a processing unit in the pipeline.
type Stage interface {
	Process(ctx context.Context, upstream <-chan interface{}) <-chan interface{}
	Close() error
}

// Pipeline wires multiple stages sequentially.
type Pipeline struct {
	stages []Stage
}

// New constructs a pipeline from the provided stages.
func New(stages ...Stage) *Pipeline {
	return &Pipeline{stages: stages}
}

// Run starts each stage sequentially, passing downstream channels along the chain.
func (p *Pipeline) Run(ctx context.Context) error {
	if len(p.stages) == 0 {
		return fmt.Errorf("pipeline has no stages")
	}
	var upstream <-chan interface{}
	for _, stage := range p.stages {
		upstream = stage.Process(ctx, upstream)
	}
	return nil
}

// Close shuts down all stages in reverse order.
func (p *Pipeline) Close() error {
	var firstErr error
	for i := len(p.stages) - 1; i >= 0; i-- {
		if err := p.stages[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
