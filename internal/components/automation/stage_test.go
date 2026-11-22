package automation

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/features/automations"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// 購読可能な記録だけで、schedule.source が一致するジョブが動き、event に記録の中身が渡る。
func TestSubscribableRecordRunsMatchingJob(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ran := make(chan automations.Delivery, 4)
	service, err := automations.New(filepath.Join(t.TempDir(), "jobs.json"), func(_ context.Context, d automations.Delivery) error {
		ran <- d
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	service.SetSources(map[string]string{"user_location": "場所が変わった"})
	if _, err := service.Create(automations.Spec{Name: "帰宅", Script: `remind("x")`, Schedule: automations.Schedule{Source: "user_location"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	s := New(service)
	go s.Stage().Run(ctx)
	created := time.Date(2026, 11, 15, 18, 0, 0, 0, time.UTC)
	record := types.ConversationRecord{Source: "user_location", Title: "自宅", Text: "変わった", Metadata: map[string]any{"place": "自宅", "previous_place": "会社"}, CreatedAt: created}
	// 購読可能でない記録や、Source の違う記録では動かない。
	s.Stage().Upstream <- types.EventConversationInput{ConversationInput: types.ConversationInput{Record: record}}
	other := record
	other.Source = "web_search"
	s.Stage().Upstream <- types.EventConversationInput{ConversationInput: types.ConversationInput{Record: other, Subscribable: true}}
	s.Stage().Upstream <- types.EventConversationInput{ConversationInput: types.ConversationInput{Record: record, Subscribable: true}}
	select {
	case d := <-ran:
		var payload map[string]any
		if err := json.Unmarshal(d.Event, &payload); err != nil {
			t.Fatal(err)
		}
		metadata, _ := payload["metadata"].(map[string]any)
		if payload["source"] != "user_location" || payload["title"] != "自宅" || payload["text"] != "変わった" ||
			metadata["place"] != "自宅" || metadata["previous_place"] != "会社" || payload["created_at"] != "2026-11-15T18:00:00Z" {
			t.Fatalf("event の中身が不正: %s", d.Event)
		}
	case <-ctx.Done():
		t.Fatal("ジョブが動きません")
	}
	select {
	case d := <-ran:
		t.Fatalf("余計にジョブが動きました: %#v", d)
	case <-time.After(100 * time.Millisecond):
	}
}

// 予約ジョブの実行結果は会話管理へ流し、購読可能にはしない。
func TestDeliverEmitsNonSubscribableInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	service, err := automations.New(filepath.Join(t.TempDir(), "jobs.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	s := New(service)
	go s.Stage().Run(ctx)
	if err := s.Deliver(ctx, types.ConversationInput{Record: types.ConversationRecord{ID: "r"}, Subscribable: true, ModelShouldRespond: true}); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-s.Stage().Downstream:
		input, ok := e.(types.EventConversationInput)
		if !ok || input.Record.ID != "r" || input.Subscribable || !input.ModelShouldRespond {
			t.Fatalf("流れたイベントが不正: %#v", e)
		}
	case <-ctx.Done():
		t.Fatal("実行結果が流れません")
	}
}
