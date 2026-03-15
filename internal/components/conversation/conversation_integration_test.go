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
	t.Run("人の発話開始で再生中assistantがcancelされる", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendTextInput("こんにちは")
		h.sendResponse(req.RequestID, `{"timeline":[{"type":"speech","text":"最初の返答です"}]}`)

		first := h.expectRealtimeOutputText("最初の返答です")
		h.expectFinalOutput(first.ResponseID)

		h.sendEvent(types.Event{Kind: types.EventSpeechStart})

		cancelEvt := h.expectEvent(types.EventTTSCancel)
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

		secondEvt := h.expectEvent(types.EventResponsesRequest)
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

		secondEvt := h.expectEvent(types.EventResponsesRequest)
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

	evt := h.expectEvent(types.EventResponsesRequest)
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

func (h *conversationHarness) sendEvent(evt types.Event) {
	h.t.Helper()
	h.stage.Upstream <- evt
}

func (h *conversationHarness) expectRealtimeOutputText(text string) types.OutputLine {
	h.t.Helper()

	evt := h.expectEvent(types.EventRealtimeOutput)
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

	evt := h.expectEvent(types.EventRealtimeOutput)
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

	evt := h.expectEvent(types.EventWhiteboardUpdate)
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

func (h *conversationHarness) expectNoEvent(wait time.Duration) {
	h.t.Helper()

	select {
	case evt := <-h.stage.Downstream:
		h.t.Fatalf("unexpected event: %s %#v", evt.Kind, evt.Payload)
	case <-time.After(wait):
	}
}
