package sessionlifecycle

import (
	"context"
	"strings"
	"testing"
	"time"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

func TestSessionLifecycle(t *testing.T) {
	t.Run("activity後にidleで日記リクエストを出す", func(t *testing.T) {
		h := newHarness(t, 40*time.Millisecond)

		snapshot := []types.ChatMessage{
			{Role: "user", Content: "今日は散歩した"},
			{Role: "assistant", Content: "いい一日だったんだね"},
		}
		h.send(types.Event{
			Kind: types.EventConversationSnapshotUpdated,
			Payload: types.ConversationSnapshot{
				Messages: snapshot,
			},
		})
		h.send(types.Event{
			Kind: types.EventConversationActivity,
			Payload: types.ConversationActivity{
				At:     time.Now(),
				Source: "human_turn_committed",
			},
		})

		evt := h.expectEvent(types.EventResponsesRequest)
		req, ok := evt.Payload.(types.ResponsesRequest)
		if !ok {
			t.Fatalf("ResponsesRequest payload type = %T", evt.Payload)
		}
		if len(req.Messages) != len(snapshot) {
			t.Fatalf("messages len = %d, want %d", len(req.Messages), len(snapshot))
		}
		if req.ToolChoice == nil {
			t.Fatal("ToolChoice is nil")
		}
		toolChoice, ok := req.ToolChoice.(map[string]any)
		if !ok {
			t.Fatalf("ToolChoice type = %T", req.ToolChoice)
		}
		if toolChoice["name"] != "write_diary" {
			t.Fatalf("tool choice name = %#v, want write_diary", toolChoice["name"])
		}
		if len(req.Tools) != 1 {
			t.Fatalf("tools len = %d, want 1", len(req.Tools))
		}
		if !strings.Contains(req.Text, shortDiaryInstruction) {
			t.Fatalf("request text = %q, want short diary instruction", req.Text)
		}
	})

	t.Run("会話量に応じて日記の文量指示を変える", func(t *testing.T) {
		tests := []struct {
			name string
			n    int
			want string
		}{
			{name: "short", n: 6, want: shortDiaryInstruction},
			{name: "medium", n: 7, want: mediumDiaryInstruction},
			{name: "long", n: 13, want: longDiaryInstruction},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var messages []types.ChatMessage
				for i := 0; i < tt.n; i++ {
					role := "user"
					if i%2 == 1 {
						role = "assistant"
					}
					messages = append(messages, types.ChatMessage{Role: role, Content: "会話"})
				}

				got := buildDiaryPrompt(messages)
				if !strings.Contains(got, tt.want) {
					t.Fatalf("buildDiaryPrompt() = %q, want instruction %q", got, tt.want)
				}
			})
		}
	})

	t.Run("文量判定ではsystem messageを数えない", func(t *testing.T) {
		messages := []types.ChatMessage{
			{Role: "system", Content: "ツール実行結果"},
			{Role: "user", Content: "短い会話"},
			{Role: "assistant", Content: "返答"},
		}

		got := buildDiaryPrompt(messages)
		if !strings.Contains(got, shortDiaryInstruction) {
			t.Fatalf("buildDiaryPrompt() = %q, want short diary instruction", got)
		}
	})

	t.Run("write_diaryの結果が返ったらsession clearを出す", func(t *testing.T) {
		h := newHarness(t, 40*time.Millisecond)

		h.send(types.Event{
			Kind: types.EventConversationSnapshotUpdated,
			Payload: types.ConversationSnapshot{
				Messages: []types.ChatMessage{{Role: "user", Content: "会話"}},
			},
		})
		h.send(types.Event{
			Kind: types.EventConversationActivity,
			Payload: types.ConversationActivity{
				At: time.Now(),
			},
		})
		h.expectEvent(types.EventResponsesRequest)

		h.send(types.Event{
			Kind: types.EventToolResponse,
			Payload: types.ToolResponse{
				Name: "write_diary",
			},
		})

		h.expectEvent(types.EventSessionClear)
	})

	t.Run("日記実行中に新しいactivityが来たら古い完了通知を無視する", func(t *testing.T) {
		h := newHarness(t, 40*time.Millisecond)

		h.send(types.Event{
			Kind: types.EventConversationSnapshotUpdated,
			Payload: types.ConversationSnapshot{
				Messages: []types.ChatMessage{{Role: "user", Content: "最初の会話"}},
			},
		})
		h.send(types.Event{
			Kind: types.EventConversationActivity,
			Payload: types.ConversationActivity{
				At: time.Now(),
			},
		})
		h.expectEvent(types.EventResponsesRequest)

		h.send(types.Event{
			Kind: types.EventConversationActivity,
			Payload: types.ConversationActivity{
				At: time.Now(),
			},
		})
		h.send(types.Event{
			Kind: types.EventToolResponse,
			Payload: types.ToolResponse{
				Name: "write_diary",
			},
		})

		h.expectNoSessionClear(30 * time.Millisecond)
		h.expectEvent(types.EventResponsesRequest)
	})
}

type harness struct {
	t     *testing.T
	stage *graph.Stage
}

func newHarness(t *testing.T, idleThreshold time.Duration) *harness {
	t.Helper()

	stage := NewStage(Config{
		IdleThreshold: idleThreshold,
		WriteDiaryTools: []any{
			map[string]any{"type": "function", "name": "write_diary"},
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	stage.Run(ctx)
	t.Cleanup(func() {
		cancel()
		_ = stage.Close()
	})
	return &harness{t: t, stage: stage}
}

func (h *harness) send(evt types.Event) {
	h.t.Helper()
	h.stage.Upstream <- evt
}

func (h *harness) expectEvent(kind types.EventKind) types.Event {
	h.t.Helper()

	select {
	case evt := <-h.stage.Downstream:
		if evt.Kind != kind {
			h.t.Fatalf("event kind = %s, want %s", evt.Kind, kind)
		}
		return evt
	case <-time.After(2 * time.Second):
		h.t.Fatalf("timed out waiting for %s", kind)
		return types.Event{}
	}
}

func (h *harness) expectNoEvent(wait time.Duration) {
	h.t.Helper()

	select {
	case evt := <-h.stage.Downstream:
		h.t.Fatalf("unexpected event: %s %#v", evt.Kind, evt.Payload)
	case <-time.After(wait):
	}
}

func (h *harness) expectNoSessionClear(wait time.Duration) {
	h.t.Helper()

	deadline := time.After(wait)
	for {
		select {
		case evt := <-h.stage.Downstream:
			if evt.Kind == types.EventSessionClear {
				h.t.Fatalf("unexpected session clear: %#v", evt.Payload)
			}
			if evt.Kind == types.EventResponsesRequest {
				continue
			}
			h.t.Fatalf("unexpected event: %s %#v", evt.Kind, evt.Payload)
		case <-deadline:
			return
		}
	}
}
