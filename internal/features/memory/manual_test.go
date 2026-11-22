package memory

import (
	"context"
	"path/filepath"
	"testing"
)

func TestManualMemoryServiceCreatesAndUpdatesContent(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "memory.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := &manualMemoryService{store: store}

	record, result, err := service.Create(context.Background(), "ユーザーは朝にコーヒーを飲む")
	if err != nil || !result.Created || record.Content != "ユーザーは朝にコーヒーを飲む" {
		t.Fatalf("Create() = %#v, %#v, %v", record, result, err)
	}
	updated, err := service.Update(context.Background(), record.ID, "ユーザーは朝に紅茶を飲む")
	if err != nil || updated.ID != record.ID || updated.Content != "ユーザーは朝に紅茶を飲む" {
		t.Fatalf("Update() = %#v, %v", updated, err)
	}
}
