package timer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/states/generation"
	timerstate "github.com/tetetratra/smart-speaker/internal/states/timer"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestToolCreateAndCancelTimerTool(t *testing.T) {
	now := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	store := timerstate.NewStoreWithClock(func() time.Time { return now })
	tool := New(Config{Store: store, Now: func() time.Time { return now }})
	cancelTool := tool.CancelTool()

	var events []types.Event
	tool.SetEventEmitter(func(evt types.Event) { events = append(events, evt) })
	out, err := tool.Run(map[string]any{
		"operation": "create",
		"at":        now.Add(time.Hour).Format(time.RFC3339),
		"action":    "起こす",
	})
	if err != nil {
		t.Fatal(err)
	}
	id, _ := out["id"].(string)
	if id == "" {
		t.Fatalf("id = %#v", out["id"])
	}
	if got := len(store.Snapshot()); got != 1 {
		t.Fatalf("Snapshot len = %d, want 1", got)
	}
	if len(events) < 2 || events[len(events)-1].Kind != types.EventTimerState {
		t.Fatalf("events = %#v, want timer state event", events)
	}

	cancelled, err := cancelTool.Run(map[string]any{"id": id})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled["cancelled"] != true {
		t.Fatalf("cancelled = %#v", cancelled)
	}
	if got := len(store.Snapshot()); got != 0 {
		t.Fatalf("Snapshot len after cancel = %d, want 0", got)
	}
}

func TestCancelToolRejectsMissingOrUnknownID(t *testing.T) {
	tool := New(Config{})
	cancelTool := tool.CancelTool()

	if _, err := cancelTool.Run(map[string]any{}); err == nil {
		t.Fatal("Run returned nil error, want missing id error")
	}
	if _, err := cancelTool.Run(map[string]any{"id": "missing"}); err == nil {
		t.Fatal("Run returned nil error, want unknown id error")
	}
}

func TestToolRejectsPastTimer(t *testing.T) {
	now := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	tool := New(Config{Now: func() time.Time { return now }})

	_, err := tool.Run(map[string]any{
		"operation": "create",
		"at":        now.Add(-time.Second).Format(time.RFC3339),
		"action":    "起こす",
	})
	if err == nil {
		t.Fatal("Run returned nil error, want past timer error")
	}
}

func TestToolDefinitionInstructsActionWithoutDelayCondition(t *testing.T) {
	tool := New(Config{})
	def := tool.Definition()
	description, _ := def["description"].(string)
	for _, want := range []string{
		"actionには",
		"時刻・遅延条件を含めず",
		`action="エアコンをつける"`,
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("description = %q, want it to contain %q", description, want)
		}
	}

	params, ok := def["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters = %#v", def["parameters"])
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", params["properties"])
	}
	operation, ok := props["operation"].(map[string]any)
	if !ok {
		t.Fatalf("operation property = %#v", props["operation"])
	}
	enum, ok := operation["enum"].([]string)
	if !ok || len(enum) != 1 || enum[0] != "create" {
		t.Fatalf("operation enum = %#v, want create only", operation["enum"])
	}
	if _, ok := props["id"]; ok {
		t.Fatalf("timer definition should not expose id: %#v", props["id"])
	}
	action, ok := props["action"].(map[string]any)
	if !ok {
		t.Fatalf("action property = %#v", props["action"])
	}
	actionDescription, _ := action["description"].(string)
	for _, want := range []string{
		"相対時刻や遅延条件は含めず",
		"期限到達時点で実行する内容だけ",
		"エアコンをつける",
	} {
		if !strings.Contains(actionDescription, want) {
			t.Fatalf("action description = %q, want it to contain %q", actionDescription, want)
		}
	}
}

func TestCancelToolDefinitionRequiresID(t *testing.T) {
	tool := New(Config{})
	def := tool.CancelTool().Definition()
	if def["name"] != cancelToolName {
		t.Fatalf("name = %v, want %s", def["name"], cancelToolName)
	}

	params, ok := def["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters = %#v", def["parameters"])
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", params["properties"])
	}
	if _, ok := props["id"]; !ok || len(props) != 1 {
		t.Fatalf("properties = %#v, want id only", props)
	}
	required, ok := params["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "id" {
		t.Fatalf("required = %#v, want id", params["required"])
	}
}

func TestToolMonitorEmitsDueTimerAsSystemCommit(t *testing.T) {
	current := time.Date(2026, 6, 3, 10, 0, 0, 0, time.UTC)
	now := func() time.Time { return current }
	store := timerstate.NewStoreWithClock(now)
	timer := store.Create(current.Add(-time.Second), "エアコンをoffにする")
	gen := generation.NewStore()
	tool := New(Config{
		Store:        store,
		Generation:   gen,
		Now:          now,
		TickInterval: 5 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan types.Event, 4)
	tool.SetContext(ctx)
	tool.SetEventEmitter(func(evt types.Event) { events <- evt })

	var sawCommit bool
	deadline := time.After(time.Second)
	for !sawCommit {
		select {
		case evt := <-events:
			if evt.Kind != types.EventConversationCommitRequest {
				continue
			}
			req := evt.Payload.(types.ConversationCommitRequest)
			if req.Role != types.RoleSystem || req.Source != toolName {
				t.Fatalf("commit = %#v", req)
			}
			if req.GenerationID != 1 {
				t.Fatalf("GenerationID = %d, want 1", req.GenerationID)
			}
			if !strings.Contains(req.Text, timer.Action) {
				t.Fatalf("Text = %q, want it to contain %q", req.Text, timer.Action)
			}
			sawCommit = true
		case <-deadline:
			t.Fatal("timeout waiting due timer commit")
		}
	}
	if got := len(store.Snapshot()); got != 0 {
		t.Fatalf("Snapshot len after due = %d, want 0", got)
	}
}
