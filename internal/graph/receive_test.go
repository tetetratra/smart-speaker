package graph_test

import (
	"context"
	"github.com/tetetratra/smart-speaker/internal/components/generationfilter"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/types"
	"strings"
	"testing"
	"time"
)

func TestFilterChecksAfterBlockedHandoff(t *testing.T) {
	st := &graph.Stage{Upstream: make(chan types.Event)}
	store := generation.NewStore()
	store.Next()
	generationfilter.Attach(st, store)
	old := types.Event{Payload: types.TimelineItem{GenerationID: 1}}
	started := make(chan struct{})
	go func() {
		close(started)
		st.Upstream <- old
		st.Upstream <- types.Event{Payload: types.TimelineItem{GenerationID: 2}}
	}()
	<-started
	store.Next()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	e, ok := st.Receive(ctx)
	if !ok || e.Payload.(types.TimelineItem).GenerationID != 2 {
		t.Fatal(e, ok)
	}
}

func TestBlockedAudioDestinationDoesNotBlockControlDelivery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source := &graph.Stage{Name: "source", Downstream: make(chan types.Event), Run: func(context.Context) {}}
	audio := &graph.Stage{Name: "busy-audio", Upstream: make(chan types.Event), Run: func(context.Context) {}}
	control := &graph.Stage{Name: "control", Upstream: make(chan types.Event), Run: func(context.Context) {}}
	g := graph.New()
	a, b, c := g.AddNode(source), g.AddNode(audio), g.AddNode(control)
	g.ConnectKinds(a, b, types.EventResponseText)
	g.ConnectKinds(a, c, types.EventGenerationChanged)
	done := make(chan struct{})
	go func() { _ = g.Run(ctx); close(done) }()
	source.Downstream <- types.Event{Kind: types.EventResponseText, Payload: types.ResponseText{GenerationID: 1, Text: "待機する音声"}}
	source.Downstream <- types.Event{Kind: types.EventGenerationChanged, Payload: types.GenerationChanged{GenerationID: 2}}
	select {
	case <-control.Upstream:
	case <-time.After(time.Second):
		t.Fatal("音声待ちで制御通知が止まった")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("終了できない")
	}
}

func TestStaleToolRequestProducesExplicitNotExecutedResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	store := generation.NewStore()
	store.Next()
	store.Next()
	st := &graph.Stage{Upstream: make(chan types.Event), Downstream: make(chan types.Event)}
	generationfilter.AttachToolRequests(st, store)
	go func() {
		st.Upstream <- types.Event{Kind: types.EventToolRequest, Payload: types.ToolRequest{ToolCallID: "old", GenerationID: 1}}
	}()
	done := make(chan struct{})
	go func() { st.Receive(ctx); close(done) }()
	select {
	case e := <-st.Downstream:
		result := e.Payload.(types.ConversationCommitRequest).ToolResult
		if result == nil || result.ToolCallID != "old" || !strings.Contains(string(result.Output), `"executed":false`) {
			t.Fatal(result)
		}
	case <-ctx.Done():
		t.Fatal("未実行の結果が届かない")
	}
	cancel()
	<-done
}
