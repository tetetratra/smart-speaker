package turnmanager

import (
	"context"
	"testing"
	"time"

	types "smart-speaker/internal/types"
)

func TestTurnManagerEmitsAfterStopAndFinal(t *testing.T) {
	stage := NewStage()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go stage.Run(ctx)

	stage.Upstream <- types.Event{Kind: types.EventVadStart, Payload: types.VadEvent{Type: "start"}}
	stage.Upstream <- types.Event{Kind: types.EventTranscriptFinal, Payload: types.TranscriptEvent{Text: "こんにちは", Final: true}}
	stage.Upstream <- types.Event{Kind: types.EventVadStop, Payload: types.VadEvent{Type: "stop"}}

	select {
	case evt := <-stage.Downstream:
		if evt.Kind != types.EventResponsesRequest {
			t.Fatalf("unexpected event kind: %v", evt.Kind)
		}
		req, ok := evt.Payload.(types.ResponsesRequest)
		if !ok {
			t.Fatalf("unexpected payload type: %T", evt.Payload)
		}
		if req.Text != "こんにちは" {
			t.Fatalf("unexpected text: %q", req.Text)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for response request")
	}
}
