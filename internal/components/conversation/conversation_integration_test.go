package conversation

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

func TestConversationIntegration(t *testing.T) {
	t.Run("人の確定発話で活動イベントと会話スナップショットが出る", func(t *testing.T) {
		h := newConversationHarness(t)

		h.sendEvent(types.Event{
			Kind: types.EventTextInput,
			Payload: types.OutputLine{
				Role: "user",
				Text: "こんにちは",
			},
		})

		activityEvt := h.expectEvent(types.EventConversationActivity)
		activity, ok := activityEvt.Payload.(types.ConversationActivity)
		if !ok {
			t.Fatalf("ConversationActivity payload type = %T", activityEvt.Payload)
		}
		if activity.Source != "human_turn_committed" {
			t.Fatalf("activity source = %q, want human_turn_committed", activity.Source)
		}

		snapshotEvt := h.expectEvent(types.EventConversationSnapshotUpdated)
		snapshot, ok := snapshotEvt.Payload.(types.ConversationSnapshot)
		if !ok {
			t.Fatalf("ConversationSnapshot payload type = %T", snapshotEvt.Payload)
		}
		if len(snapshot.Messages) != 1 {
			t.Fatalf("snapshot messages len = %d, want 1", len(snapshot.Messages))
		}
		if snapshot.Messages[0].Role != "user" || snapshot.Messages[0].Content != "こんにちは" {
			t.Fatalf("snapshot message = %+v", snapshot.Messages[0])
		}

		h.expectMainEvent(types.EventResponsesRequest)
	})

	t.Run("人の発話開始で再生中assistantがcancelされる", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendTextInput("こんにちは")
		h.sendResponse(req.RequestID, `{"timeline":[{"type":"speech","text":"最初の返答です"}]}`)

		first := h.expectRealtimeOutputText("最初の返答です")
		h.expectFinalOutput(first.ResponseID)

		h.sendEvent(types.Event{Kind: types.EventSpeechStart})

		cancelEvt := h.expectMainEvent(types.EventTTSCancel)
		cancel, ok := cancelEvt.Payload.(types.TTSCancel)
		if !ok {
			t.Fatalf("TTSCancel payload type = %T", cancelEvt.Payload)
		}
		if cancel.ResponseID != first.ResponseID {
			t.Fatalf("cancel response_id = %q, want %q", cancel.ResponseID, first.ResponseID)
		}
	})

	t.Run("人の確定発話でResponsesRequestが1回出る", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendTextInput("音量を上げて")

		if req.RequestID == "" {
			t.Fatal("ResponsesRequest.RequestID is empty")
		}
		if len(req.Messages) == 0 {
			t.Fatal("ResponsesRequest.Messages is empty")
		}
		last := req.Messages[len(req.Messages)-1]
		if last.Role != "user" || last.Content != "音量を上げて" {
			t.Fatalf("last message = %+v, want user message", last)
		}

		h.expectNoEvent(150 * time.Millisecond)
	})

	t.Run("invalid responseでretryが1回だけ走る", func(t *testing.T) {
		h := newConversationHarness(t)

		first := h.sendTextInput("テスト")
		h.sendResponse(first.RequestID, `{"timeline":[}`)

		secondEvt := h.expectMainEvent(types.EventResponsesRequest)
		second, ok := secondEvt.Payload.(types.ResponsesRequest)
		if !ok {
			t.Fatalf("retry payload type = %T", secondEvt.Payload)
		}
		if second.RequestID == "" {
			t.Fatal("retry RequestID is empty")
		}
		if second.RequestID == first.RequestID {
			t.Fatalf("retry RequestID = %q, want new request id", second.RequestID)
		}

		h.sendResponse(second.RequestID, `{"timeline":[}`)
		h.expectNoEvent(150 * time.Millisecond)
	})

	t.Run("waitとspeechを含む応答でTTS完了後に次発話へ進む", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendTextInput("次の発話を待って")
		h.sendResponse(req.RequestID, `{"timeline":[{"type":"speech","text":"ひとつめ"},{"type":"wait","sec":1},{"type":"speech","text":"ふたつめ"}]}`)

		first := h.expectRealtimeOutputText("ひとつめ")
		h.expectFinalOutput(first.ResponseID)

		h.sendEvent(types.Event{
			Kind: types.EventTTSEnd,
			Payload: types.TTSEvent{
				ResponseID:      first.ResponseID,
				AudioStartAt:    time.Now().Add(-2 * time.Second),
				DurationSeconds: 1,
			},
		})

		h.expectRealtimeOutputText("ふたつめ")
	})

	t.Run("レスポンスにwhiteboardが含まれる場合は白板更新イベントも出る", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendTextInput("明日の予定を教えて")
		h.sendResponse(req.RequestID, `{"timeline":[{"type":"speech","text":"明日の予定を確認したよ"}],"whiteboard":{"content":"- 10:00 定例会議"}}`)

		h.expectWhiteboardUpdate("- 10:00 定例会議")
		first := h.expectRealtimeOutputText("明日の予定を確認したよ")
		h.expectFinalOutput(first.ResponseID)
	})

	t.Run("レスポンスにwhiteboardがない場合は白板更新イベントが出ない", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendTextInput("こんにちは")
		h.sendResponse(req.RequestID, `{"timeline":[{"type":"speech","text":"やあ"}]}`)

		first := h.expectRealtimeOutputText("やあ")
		h.expectFinalOutput(first.ResponseID)
		h.expectNoEvent(150 * time.Millisecond)
	})

	t.Run("whiteboardが不正な形式の場合はinvalid responseとして再試行する", func(t *testing.T) {
		h := newConversationHarness(t)

		first := h.sendTextInput("予定を教えて")
		h.sendResponse(first.RequestID, `{"timeline":[{"type":"speech","text":"確認したよ"}],"whiteboard":{"content":"   "}}`)

		secondEvt := h.expectMainEvent(types.EventResponsesRequest)
		second, ok := secondEvt.Payload.(types.ResponsesRequest)
		if !ok {
			t.Fatalf("retry payload type = %T", secondEvt.Payload)
		}
		if second.RequestID == "" {
			t.Fatal("retry RequestID is empty")
		}
		if second.RequestID == first.RequestID {
			t.Fatalf("retry RequestID = %q, want new request id", second.RequestID)
		}
	})

	t.Run("streaming speech chunk到着時点で発話を開始する", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendTextInput("こんにちは")
		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"やあ"}`)

		first := h.expectRealtimeOutputText("やあ")
		h.expectFinalOutput(first.ResponseID)
	})

	t.Run("streaming wait後のspeechはtimer経過まで発話しない", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendTextInput("少し待って")
		h.sendStreamLine(req.RequestID, `{"type":"wait","sec":1}`)
		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"待ったよ"}`)

		h.expectNoEvent(150 * time.Millisecond)
		first := h.expectRealtimeOutputText("待ったよ")
		h.expectFinalOutput(first.ResponseID)
	})

	t.Run("streaming whiteboard chunkは到着時点で白板更新する", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendTextInput("明日の予定")
		h.sendStreamLine(req.RequestID, `{"type":"whiteboard","content":"- 10:00 会議"}`)
		h.expectWhiteboardUpdate("- 10:00 会議")
	})

	t.Run("streaming中はTTS完了後もdoneまで後続chunkを待つ", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendTextInput("続けて")
		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"ひとつめ"}`)
		first := h.expectRealtimeOutputText("ひとつめ")
		h.expectFinalOutput(first.ResponseID)

		h.sendEvent(types.Event{
			Kind: types.EventTTSEnd,
			Payload: types.TTSEvent{
				ResponseID:      first.ResponseID,
				AudioStartAt:    time.Now().Add(-2 * time.Second),
				DurationSeconds: 1,
			},
		})
		h.expectNoEvent(150 * time.Millisecond)

		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"ふたつめ"}`)
		second := h.expectRealtimeOutputText("ふたつめ")
		h.expectFinalOutput(second.ResponseID)
	})

	t.Run("streaming invalid chunkが発話前なら再試行する", func(t *testing.T) {
		h := newConversationHarness(t)

		first := h.sendTextInput("テスト")
		h.sendStreamLine(first.RequestID, `{"type":"speech","text":" "}`)

		secondEvt := h.expectMainEvent(types.EventResponsesRequest)
		second, ok := secondEvt.Payload.(types.ResponsesRequest)
		if !ok {
			t.Fatalf("retry payload type = %T", secondEvt.Payload)
		}
		if second.RequestID == "" {
			t.Fatal("retry RequestID is empty")
		}
		if second.RequestID == first.RequestID {
			t.Fatalf("retry RequestID = %q, want new request id", second.RequestID)
		}
	})

	t.Run("streaming doneまでspeechがなければ再試行する", func(t *testing.T) {
		h := newConversationHarness(t)

		first := h.sendTextInput("テスト")
		h.sendStreamDone(first.RequestID)

		secondEvt := h.expectMainEvent(types.EventResponsesRequest)
		second, ok := secondEvt.Payload.(types.ResponsesRequest)
		if !ok {
			t.Fatalf("retry payload type = %T", secondEvt.Payload)
		}
		if second.RequestID == "" || second.RequestID == first.RequestID {
			t.Fatalf("retry RequestID = %q, first = %q", second.RequestID, first.RequestID)
		}
	})

	t.Run("streaming done後は会話スナップショットを更新する", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendTextInput("こんにちは")
		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"やあ"}`)
		first := h.expectRealtimeOutputText("やあ")
		h.expectFinalOutput(first.ResponseID)
		h.sendEvent(types.Event{
			Kind: types.EventTTSEnd,
			Payload: types.TTSEvent{
				ResponseID:      first.ResponseID,
				AudioStartAt:    time.Now().Add(-2 * time.Second),
				DurationSeconds: 1,
			},
		})
		h.sendStreamDone(req.RequestID)
		h.expectEvent(types.EventConversationSnapshotUpdated)
	})

	t.Run("streaming invalid chunkが発話後なら再試行しない", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendTextInput("テスト")
		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"先に話す"}`)
		first := h.expectRealtimeOutputText("先に話す")
		h.expectFinalOutput(first.ResponseID)

		h.sendStreamLine(req.RequestID, `{"type":"unknown"}`)
		h.expectNoEvent(150 * time.Millisecond)
	})
}

type conversationHarness struct {
	t     *testing.T
	stage *graph.Stage
}

func newConversationHarness(t *testing.T) *conversationHarness {
	t.Helper()

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("GOOGLE_OAUTH_TOKEN_PATH", filepath.Join(tmpDir, "missing-google-token.json"))

	stage := NewStage(Config{})
	ctx, cancel := context.WithCancel(context.Background())
	stage.Run(ctx)
	t.Cleanup(func() {
		cancel()
		_ = stage.Close()
	})

	return &conversationHarness{
		t:     t,
		stage: stage,
	}
}

func (h *conversationHarness) sendTextInput(text string) types.ResponsesRequest {
	h.t.Helper()

	h.sendEvent(types.Event{
		Kind: types.EventTextInput,
		Payload: types.OutputLine{
			Role: "user",
			Text: text,
		},
	})

	evt := h.expectMainEvent(types.EventResponsesRequest)
	req, ok := evt.Payload.(types.ResponsesRequest)
	if !ok {
		h.t.Fatalf("ResponsesRequest payload type = %T", evt.Payload)
	}
	return req
}

func (h *conversationHarness) sendResponse(requestID string, raw string) {
	h.t.Helper()
	h.sendEvent(types.Event{
		Kind: types.EventResponsesResponse,
		Payload: types.ResponsesResponse{
			RequestID:   requestID,
			Text:        raw,
			HasResponse: true,
		},
	})
}

func (h *conversationHarness) sendStreamLine(requestID string, line string) {
	h.t.Helper()
	h.sendEvent(types.Event{
		Kind: types.EventResponsesStreamChunk,
		Payload: types.ResponsesStreamChunk{
			RequestID: requestID,
			Line:      line,
		},
	})
}

func (h *conversationHarness) sendStreamDone(requestID string) {
	h.t.Helper()
	h.sendEvent(types.Event{
		Kind: types.EventResponsesStreamChunk,
		Payload: types.ResponsesStreamChunk{
			RequestID: requestID,
			Done:      true,
		},
	})
}

func (h *conversationHarness) sendEvent(evt types.Event) {
	h.t.Helper()
	h.stage.Upstream <- evt
}

func (h *conversationHarness) expectRealtimeOutputText(text string) types.OutputLine {
	h.t.Helper()

	evt := h.expectMainEvent(types.EventRealtimeOutput)
	line, ok := evt.Payload.(types.OutputLine)
	if !ok {
		h.t.Fatalf("RealtimeOutput payload type = %T", evt.Payload)
	}
	if line.Text != text {
		h.t.Fatalf("RealtimeOutput text = %q, want %q", line.Text, text)
	}
	if line.ResponseID == "" {
		h.t.Fatal("RealtimeOutput response_id is empty")
	}
	return line
}

func (h *conversationHarness) expectFinalOutput(responseID string) {
	h.t.Helper()

	evt := h.expectMainEvent(types.EventRealtimeOutput)
	line, ok := evt.Payload.(types.OutputLine)
	if !ok {
		h.t.Fatalf("final RealtimeOutput payload type = %T", evt.Payload)
	}
	if !line.Final {
		h.t.Fatalf("final output Final = false, payload = %+v", line)
	}
	if line.ResponseID != responseID {
		h.t.Fatalf("final output response_id = %q, want %q", line.ResponseID, responseID)
	}
}

func (h *conversationHarness) expectWhiteboardUpdate(content string) {
	h.t.Helper()

	evt := h.expectMainEvent(types.EventWhiteboardUpdate)
	update, ok := evt.Payload.(types.WhiteboardUpdate)
	if !ok {
		h.t.Fatalf("WhiteboardUpdate payload type = %T", evt.Payload)
	}
	if update.Content != content {
		h.t.Fatalf("whiteboard content = %q, want %q", update.Content, content)
	}
}

func (h *conversationHarness) expectEvent(kind types.EventKind) types.Event {
	h.t.Helper()

	select {
	case evt := <-h.stage.Downstream:
		if evt.Kind != kind {
			h.t.Fatalf("event kind = %s, want %s", evt.Kind, kind)
		}
		return evt
	case <-time.After(3 * time.Second):
		h.t.Fatalf("timed out waiting for %s", kind)
		return types.Event{}
	}
}

func (h *conversationHarness) expectMainEvent(kind types.EventKind) types.Event {
	h.t.Helper()

	for {
		evt := h.expectAnyEvent()
		if evt.Kind == kind {
			return evt
		}
		switch evt.Kind {
		case types.EventConversationActivity, types.EventConversationSnapshotUpdated:
			continue
		default:
			h.t.Fatalf("event kind = %s, want %s", evt.Kind, kind)
		}
	}
}

func (h *conversationHarness) expectNoEvent(wait time.Duration) {
	h.t.Helper()

	select {
	case evt := <-h.stage.Downstream:
		switch evt.Kind {
		case types.EventConversationActivity, types.EventConversationSnapshotUpdated:
			h.expectNoEvent(wait)
		default:
			h.t.Fatalf("unexpected event: %s %#v", evt.Kind, evt.Payload)
		}
	case <-time.After(wait):
	}
}

func (h *conversationHarness) expectAnyEvent() types.Event {
	h.t.Helper()

	select {
	case evt := <-h.stage.Downstream:
		return evt
	case <-time.After(3 * time.Second):
		h.t.Fatal("timed out waiting for event")
		return types.Event{}
	}
}
