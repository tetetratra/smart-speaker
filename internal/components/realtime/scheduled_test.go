package realtime

import (
	"context"
	"github.com/tetetratra/smart-speaker/internal/types"
	"strings"
	"testing"
	"time"
)

func TestQuietScheduledJobDoesNotEmitSpeechAndNormalSpeechStillWorks(t *testing.T) {
	scheduled := make(chan types.ScheduledPrompt, 1)
	r := newRig(t, func(cfg *Config) { cfg.Scheduled = scheduled })
	scheduled <- types.ScheduledPrompt{ID: "job1", Name: "無音の仕事モード", Instruction: "状態を切り替えて", Speak: false}
	item := r.wait("conversation.item.create")["item"].(map[string]any)
	text := item["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "状態を切り替えて") || !strings.Contains(text, "音声通知不要") {
		t.Fatal(text)
	}
	req := r.wait("response.create")
	r.response("scheduled-response", "scheduled-item", "設定を変更しました。", req["response"].(map[string]any)["metadata"])
	for {
		event := r.event(types.EventRealtimeOutput)
		if event.Payload.(types.OutputLine).Role == types.RoleAgent {
			break
		}
	}
	select {
	case event := <-r.events:
		if event.Kind == types.EventResponseText {
			t.Fatal("無音のジョブが音声合成へ渡りました")
		}
	case <-time.After(30 * time.Millisecond):
	}
	records := r.history.Snapshot()
	found := false
	for _, record := range records {
		if record.Role == types.RoleAgent && record.Metadata["silent"] == true {
			found = true
		}
	}
	if !found {
		t.Fatal(records)
	}
	// 音声を意図的に出さなかった履歴を、中断扱いで修復してはならない。
	r.node.Upstream <- types.Event{Payload: types.LLMRequest{Text: "普通の質問"}}
	nextItem := r.wait("conversation.item.create")["item"].(map[string]any)
	if nextItem["role"] != "user" {
		t.Fatal(nextItem)
	}
	req = r.wait("response.create")
	r.response("normal-response", "normal-item", "普通の回答。", req["response"].(map[string]any)["metadata"])
	if e := r.event(types.EventResponseText).Payload.(types.ResponseText); e.Text != "普通の回答。" {
		t.Fatal(e)
	}
}
func TestScheduledPromptWaitsForConversationAndCoalescesSameJob(t *testing.T) {
	s := &stage{speaking: true}
	s.queueScheduled(types.ScheduledPrompt{ID: "same", Instruction: "前の時刻"})
	s.queueScheduled(types.ScheduledPrompt{ID: "same", Instruction: "最新の時刻"})
	if s.startScheduled(context.Background()) || len(s.scheduled) != 1 || s.scheduled[0].Instruction != "最新の時刻" {
		t.Fatal(s.scheduled)
	}
}
