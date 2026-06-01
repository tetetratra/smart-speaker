package interimstopper

import (
	"context"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/generation"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestStageStopsOncePerUtteranceAndPassesFinal(t *testing.T) {
	store := generation.NewStore()
	st := NewStage(Config{Generation: store})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- interim("明日")
	st.Upstream <- interim("明日の")
	if !waitForGeneration(store, 1) {
		got := store.Current()
		t.Fatalf("generation after repeated interim = %d, want 1", got)
	}
	assertNoEvent(t, st.Downstream)

	st.Upstream <- final("明日の予定")
	evt := expectEvent(t, st.Downstream)
	if evt.Kind != types.EventHumanUtterance {
		t.Fatalf("Kind = %s, want EventHumanUtterance", evt.Kind)
	}
	line := evt.Payload.(types.OutputLine)
	if line.Text != "明日の予定" {
		t.Fatalf("Text = %q, want 明日の予定", line.Text)
	}

	st.Upstream <- interim("天気")
	if !waitForGeneration(store, 2) {
		got := store.Current()
		t.Fatalf("generation after next utterance interim = %d, want 2", got)
	}
}

func TestStagePassesFinalWithNilGeneration(t *testing.T) {
	st := NewStage(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	st.Run(ctx)
	defer st.Close()

	st.Upstream <- interim("止める")
	assertNoEvent(t, st.Downstream)

	st.Upstream <- final("止める")
	evt := expectEvent(t, st.Downstream)
	if evt.Kind != types.EventHumanUtterance {
		t.Fatalf("Kind = %s, want EventHumanUtterance", evt.Kind)
	}
}

func interim(text string) types.Event {
	return types.Event{
		Kind: types.EventHumanInterimUtterance,
		Payload: types.OutputLine{
			Role:   "user",
			Text:   text,
			Final:  false,
			Source: "server-stt",
		},
	}
}

func final(text string) types.Event {
	return types.Event{
		Kind: types.EventHumanUtterance,
		Payload: types.OutputLine{
			Role:   "user",
			Text:   text,
			Final:  true,
			Source: "server-stt",
		},
	}
}

func expectEvent(t *testing.T, ch <-chan types.Event) types.Event {
	t.Helper()
	select {
	case evt := <-ch:
		return evt
	case <-time.After(time.Second):
		t.Fatal("timeout waiting event")
		return types.Event{}
	}
}

func assertNoEvent(t *testing.T, ch <-chan types.Event) {
	t.Helper()
	select {
	case evt := <-ch:
		t.Fatalf("unexpected event: %#v", evt)
	case <-time.After(20 * time.Millisecond):
	}
}

func waitForGeneration(store *generation.Store, want types.GenerationID) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if store.Current() == want {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return store.Current() == want
}
