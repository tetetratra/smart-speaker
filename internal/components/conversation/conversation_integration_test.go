package conversation

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

func TestConversationIntegration(t *testing.T) {
	t.Run("人の確定発話でResponsesRequestが出る", func(t *testing.T) {
		h := newConversationHarness(t)

		h.sendEvent(types.Event{
			Kind: types.EventHumanUtterance,
			Payload: types.OutputLine{
				Role: "user",
				Text: "こんにちは",
			},
		})

		reqEvt := h.expectMainEvent(types.EventResponsesRequest)
		req, ok := reqEvt.Payload.(types.ResponsesRequest)
		if !ok {
			t.Fatalf("ResponsesRequest payload type = %T", reqEvt.Payload)
		}
		if len(req.Messages) != 1 {
			t.Fatalf("messages len = %d, want 1", len(req.Messages))
		}
		if req.Messages[0].Role != "user" || req.Messages[0].Content != "こんにちは" {
			t.Fatalf("message = %+v", req.Messages[0])
		}
	})

	t.Run("人の確定発話でResponsesRequestが1回出る", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendHumanUtterance("音量を上げて")

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

	t.Run("TTS完了前の追い質問でも直前assistant発話を会話履歴に含める", func(t *testing.T) {
		h := newConversationHarness(t)

		first := h.sendHumanUtterance("明日の予定を教えて")
		h.sendStreamLine(first.RequestID, `{"type":"speech","text":"明日は10時から会議があります"}`)

		firstOutput := h.expectRealtimeOutputText("明日は10時から会議があります")
		h.expectFinalOutput(firstOutput.ResponseID)

		h.sendEvent(types.Event{
			Kind: types.EventHumanUtterance,
			Payload: types.OutputLine{
				Role: "user",
				Text: "それについて調べて教えて",
			},
		})
		secondEvt := h.expectMainEvent(types.EventResponsesRequest)
		second, ok := secondEvt.Payload.(types.ResponsesRequest)
		if !ok {
			t.Fatalf("ResponsesRequest payload type = %T", secondEvt.Payload)
		}
		if len(second.Messages) != 3 {
			t.Fatalf("messages len = %d, want 3", len(second.Messages))
		}
		if second.Messages[0].Role != "user" || second.Messages[0].Content != "明日の予定を教えて" {
			t.Fatalf("first message = %+v", second.Messages[0])
		}
		if second.Messages[1].Role != "assistant" || second.Messages[1].Content != "明日は10時から会議があります" {
			t.Fatalf("second message = %+v", second.Messages[1])
		}
		if second.Messages[2].Role != "user" || second.Messages[2].Content != "それについて調べて教えて" {
			t.Fatalf("third message = %+v", second.Messages[2])
		}
	})

	t.Run("invalid responseでretryが1回だけ走る", func(t *testing.T) {
		h := newConversationHarness(t)

		first := h.sendHumanUtterance("テスト")
		h.sendStreamLine(first.RequestID, `{"timeline":[}`)

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
		assertRetryHintFirst(t, second.Messages, `{"timeline":[}`)

		h.sendStreamLine(second.RequestID, `{"timeline":[}`)
		h.expectNoEvent(150 * time.Millisecond)
	})

	t.Run("NDJSON応答を順番に解釈できる", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendHumanUtterance("こんにちは")
		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"やあ"}`)
		h.sendStreamLine(req.RequestID, `{"type":"wait","sec":1}`)
		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"元気？"}`)

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
		second := h.expectRealtimeOutputText("元気？")
		h.expectFinalOutput(second.ResponseID)
	})

	t.Run("waitとspeechを含む応答でTTS完了後に次発話へ進む", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendHumanUtterance("次の発話を待って")
		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"ひとつめ"}`)
		h.sendStreamLine(req.RequestID, `{"type":"wait","sec":1}`)
		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"ふたつめ"}`)

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

	t.Run("レスポンスにwhiteboard chunkが含まれる場合はinvalid responseとして再試行する", func(t *testing.T) {
		h := newConversationHarness(t)

		first := h.sendHumanUtterance("明日の予定を教えて")
		h.sendStreamLine(first.RequestID, `{"type":"whiteboard","content":"- 10:00 定例会議"}`)

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
		assertRetryHintFirst(t, second.Messages, `{"type":"whiteboard","content":"- 10:00 定例会議"}`)
	})

	t.Run("レスポンスにwhiteboardがない場合は白板更新イベントが出ない", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendHumanUtterance("こんにちは")
		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"やあ"}`)

		first := h.expectRealtimeOutputText("やあ")
		h.expectFinalOutput(first.ResponseID)
		h.expectNoEvent(150 * time.Millisecond)
	})

	t.Run("streaming speech chunk到着時点で発話を開始する", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendHumanUtterance("こんにちは")
		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"やあ"}`)

		first := h.expectRealtimeOutputText("やあ")
		h.expectFinalOutput(first.ResponseID)
	})

	t.Run("streaming wait後のspeechはtimer経過まで発話しない", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendHumanUtterance("少し待って")
		h.sendStreamLine(req.RequestID, `{"type":"wait","sec":1}`)
		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"待ったよ"}`)

		h.expectNoEvent(150 * time.Millisecond)
		first := h.expectRealtimeOutputText("待ったよ")
		h.expectFinalOutput(first.ResponseID)
	})

	t.Run("streaming whiteboard chunkはinvalid responseとして再試行する", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendHumanUtterance("明日の予定")
		h.sendStreamLine(req.RequestID, `{"type":"whiteboard","content":"- 10:00 会議"}`)

		secondEvt := h.expectMainEvent(types.EventResponsesRequest)
		second, ok := secondEvt.Payload.(types.ResponsesRequest)
		if !ok {
			t.Fatalf("retry payload type = %T", secondEvt.Payload)
		}
		if second.RequestID == "" {
			t.Fatal("retry RequestID is empty")
		}
		if second.RequestID == req.RequestID {
			t.Fatalf("retry RequestID = %q, want new request id", second.RequestID)
		}
		assertRetryHintFirst(t, second.Messages, `{"type":"whiteboard","content":"- 10:00 会議"}`)
	})

	t.Run("streaming中はTTS完了後もdoneまで後続chunkを待つ", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendHumanUtterance("続けて")
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

		first := h.sendHumanUtterance("テスト")
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
		assertRetryHintFirst(t, second.Messages, `{"type":"speech","text":" "}`)
	})

	t.Run("streaming timeline objectはinvalid responseとして再試行する", func(t *testing.T) {
		h := newConversationHarness(t)

		first := h.sendHumanUtterance("テスト")
		h.sendStreamLine(first.RequestID, `{"timeline":[{"type":"speech","text":"ひとつめ"},{"type":"speech","text":"ふたつめ"}]}`)

		secondEvt := h.expectMainEvent(types.EventResponsesRequest)
		second, ok := secondEvt.Payload.(types.ResponsesRequest)
		if !ok {
			t.Fatalf("retry payload type = %T", secondEvt.Payload)
		}
		if second.RequestID == "" || second.RequestID == first.RequestID {
			t.Fatalf("retry RequestID = %q, first = %q", second.RequestID, first.RequestID)
		}
		assertRetryHintFirst(t, second.Messages, `{"timeline":[{"type":"speech","text":"ひとつめ"},{"type":"speech","text":"ふたつめ"}]}`)
	})

	t.Run("streaming plain textはinvalid responseとして再試行する", func(t *testing.T) {
		h := newConversationHarness(t)

		first := h.sendHumanUtterance("テスト")
		h.sendStreamLine(first.RequestID, `うん、わかった、だよ`)

		secondEvt := h.expectMainEvent(types.EventResponsesRequest)
		second, ok := secondEvt.Payload.(types.ResponsesRequest)
		if !ok {
			t.Fatalf("retry payload type = %T", secondEvt.Payload)
		}
		if second.RequestID == "" || second.RequestID == first.RequestID {
			t.Fatalf("retry RequestID = %q, first = %q", second.RequestID, first.RequestID)
		}
		assertRetryHintFirst(t, second.Messages, `うん、わかった、だよ`)
	})

	t.Run("streaming doneまでspeechがなければ再試行する", func(t *testing.T) {
		h := newConversationHarness(t)

		first := h.sendHumanUtterance("テスト")
		h.sendStreamDone(first.RequestID)

		secondEvt := h.expectMainEvent(types.EventResponsesRequest)
		second, ok := secondEvt.Payload.(types.ResponsesRequest)
		if !ok {
			t.Fatalf("retry payload type = %T", secondEvt.Payload)
		}
		if second.RequestID == "" || second.RequestID == first.RequestID {
			t.Fatalf("retry RequestID = %q, first = %q", second.RequestID, first.RequestID)
		}
		assertRetryHintFirst(t, second.Messages, "(空)")
	})

	t.Run("streaming done後は追加イベントを出さない", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendHumanUtterance("こんにちは")
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
		h.expectNoEvent(150 * time.Millisecond)
	})

	t.Run("streaming invalid chunkが発話後なら再試行しない", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendHumanUtterance("テスト")
		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"先に話す"}`)
		first := h.expectRealtimeOutputText("先に話す")
		h.expectFinalOutput(first.ResponseID)

		h.sendStreamLine(req.RequestID, `{"type":"unknown"}`)
		h.expectNoEvent(150 * time.Millisecond)
	})

	t.Run("toolは先行speechの再生開始後かつTTS完了後に実行する", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendHumanUtterance("温度を確認して")
		h.sendStreamLine(req.RequestID, `{"type":"speech","text":"確認するね"}`)
		first := h.expectRealtimeOutputText("確認するね")
		h.expectFinalOutput(first.ResponseID)

		h.sendStreamLine(req.RequestID, `{"type":"tool","name":"get_temp","args":{"room":"living"}}`)
		h.expectNoEvent(150 * time.Millisecond)

		h.sendEvent(types.Event{
			Kind: types.EventTTSEnd,
			Payload: types.TTSEvent{
				ResponseID:      first.ResponseID,
				AudioStartAt:    time.Now().Add(-2 * time.Second),
				DurationSeconds: 1,
			},
		})
		toolEvt := h.expectMainEvent(types.EventToolRequest)
		toolReq, ok := toolEvt.Payload.(types.ToolRequest)
		if !ok {
			t.Fatalf("ToolRequest payload type = %T", toolEvt.Payload)
		}
		if toolReq.Name != "get_temp" {
			t.Fatalf("tool name = %q, want get_temp", toolReq.Name)
		}
		if toolReq.GenerationID == 0 {
			t.Fatal("tool generation_id is empty")
		}
		if string(toolReq.Arguments) != `{"room":"living"}` {
			t.Fatalf("tool args = %s", toolReq.Arguments)
		}
	})

	t.Run("tool結果は履歴に保存されてLLM再実行を起動する", func(t *testing.T) {
		h := newConversationHarness(t)

		req := h.sendHumanUtterance("温度を確認して")
		h.sendStreamLine(req.RequestID, `{"type":"tool","name":"get_temp","args":{"room":"living"}}`)
		toolEvt := h.expectMainEvent(types.EventToolRequest)
		toolReq := toolEvt.Payload.(types.ToolRequest)

		h.commitToolResult(types.ToolResponse{
			ToolCallID:   toolReq.ToolCallID,
			Name:         toolReq.Name,
			Output:       json.RawMessage(`{"temperature":28}`),
			GenerationID: toolReq.GenerationID,
		})

		nextEvt := h.expectMainEvent(types.EventResponsesRequest)
		nextReq, ok := nextEvt.Payload.(types.ResponsesRequest)
		if !ok {
			t.Fatalf("ResponsesRequest payload type = %T", nextEvt.Payload)
		}
		last := nextReq.Messages[len(nextReq.Messages)-1]
		if last.Role != "system" {
			t.Fatalf("last role = %q, want system", last.Role)
		}
		for _, want := range []string{"ツール実行結果", "name=get_temp", "stale=false", `"temperature":28`} {
			if !strings.Contains(last.Content, want) {
				t.Fatalf("tool result message = %q, want contains %q", last.Content, want)
			}
		}
	})

	t.Run("古い世代のtool結果もstale情報付きでLLMへ渡す", func(t *testing.T) {
		h := newConversationHarness(t)

		h.sendHumanUtterance("温度を確認して")
		second := h.sendHumanUtterance("別の話をしよう")

		h.commitToolResult(types.ToolResponse{
			ToolCallID:   "tool_call_old",
			Name:         "get_temp",
			Output:       json.RawMessage(`{"temperature":27}`),
			GenerationID: 1,
		})

		nextEvt := h.expectMainEvent(types.EventResponsesRequest)
		nextReq, ok := nextEvt.Payload.(types.ResponsesRequest)
		if !ok {
			t.Fatalf("ResponsesRequest payload type = %T", nextEvt.Payload)
		}
		if nextReq.RequestID == second.RequestID {
			t.Fatalf("tool result request reused second request_id = %q", nextReq.RequestID)
		}
		last := nextReq.Messages[len(nextReq.Messages)-1]
		for _, want := range []string{"stale=true", "generation_id=1", "current_generation_id=2"} {
			if !strings.Contains(last.Content, want) {
				t.Fatalf("tool result message = %q, want contains %q", last.Content, want)
			}
		}
	})
}

type conversationHarness struct {
	t     *testing.T
	stage *graph.Stage
	sink  *ToolResultSink
}

func newConversationHarness(t *testing.T) *conversationHarness {
	t.Helper()

	tmpDir := t.TempDir()
	t.Chdir(tmpDir)
	t.Setenv("GOOGLE_OAUTH_TOKEN_PATH", filepath.Join(tmpDir, "missing-google-token.json"))

	sink := NewToolResultSink()
	stage := NewStage(Config{ToolResults: sink})
	ctx, cancel := context.WithCancel(context.Background())
	stage.Run(ctx)
	t.Cleanup(func() {
		cancel()
		_ = stage.Close()
	})

	return &conversationHarness{
		t:     t,
		stage: stage,
		sink:  sink,
	}
}

func (h *conversationHarness) sendHumanUtterance(text string) types.ResponsesRequest {
	h.t.Helper()

	h.sendEvent(types.Event{
		Kind: types.EventHumanUtterance,
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

func (h *conversationHarness) commitToolResult(resp types.ToolResponse) {
	h.t.Helper()
	h.sink.Commit(context.Background(), resp)
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

func assertRetryHintFirst(t *testing.T, messages []types.ChatMessage, invalidRaw string) {
	t.Helper()
	if len(messages) == 0 {
		t.Fatal("retry messages are empty")
	}
	first := messages[0]
	if first.Role != "system" {
		t.Fatalf("retry first role = %q, want system", first.Role)
	}
	if !strings.HasPrefix(first.Content, importantRetryPrefix) {
		t.Fatalf("retry first message = %q, want important retry prefix", first.Content)
	}
	if !strings.Contains(first.Content, "NDJSON") {
		t.Fatalf("retry first message = %q, want NDJSON guidance", first.Content)
	}
	if !strings.Contains(first.Content, "契約違反") {
		t.Fatalf("retry first message = %q, want contract violation note", first.Content)
	}
	quoted, err := json.Marshal(invalidRaw)
	if err != nil {
		t.Fatalf("json.Marshal(%q): %v", invalidRaw, err)
	}
	if !strings.Contains(first.Content, string(quoted)) {
		t.Fatalf("retry first message = %q, want invalid raw %q", first.Content, invalidRaw)
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
		h.t.Fatalf("event kind = %s, want %s", evt.Kind, kind)
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
