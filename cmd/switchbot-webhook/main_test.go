package main

import (
	"context"
	"errors"
	"io"
	"testing"
)

type fakeWebhookClient struct {
	urls    []string
	err     error
	changes []string
}

func (c *fakeWebhookClient) WebhookURLs(context.Context) ([]string, error) { return c.urls, c.err }
func (c *fakeWebhookClient) RegisterWebhook(_ context.Context, u string) error {
	c.changes = append(c.changes, "register "+u)
	return nil
}
func (c *fakeWebhookClient) DeleteWebhook(_ context.Context, u string) error {
	c.changes = append(c.changes, "delete "+u)
	return nil
}

// 外部の送信先を変える操作では、照会できない場合や別の登録がある場合に変更しない。
func TestApplyPreservesOtherWebhook(t *testing.T) {
	for _, tt := range []struct {
		name, action string
		urls         []string
		err          error
		wantErr      bool
		changes      int
	}{
		{name: "新規登録", action: "register", changes: 1},
		{name: "同じ登録", action: "register", urls: []string{"https://ours"}},
		{name: "別の登録", action: "register", urls: []string{"https://other"}, wantErr: true},
		{name: "照会失敗", action: "register", err: errors.New("error"), wantErr: true},
		{name: "自分の解除", action: "delete", urls: []string{"https://ours", "https://other"}, changes: 1},
		{name: "別の解除", action: "delete", urls: []string{"https://other"}, wantErr: true},
		{name: "照会だけ", action: "query", urls: []string{"https://ours"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeWebhookClient{urls: tt.urls, err: tt.err}
			err := apply(context.Background(), client, tt.action, "https://ours", io.Discard)
			if (err != nil) != tt.wantErr || len(client.changes) != tt.changes {
				t.Fatalf("err=%v changes=%v", err, client.changes)
			}
			for _, change := range client.changes {
				if change != tt.action+" https://ours" {
					t.Fatal(change)
				}
			}
		})
	}
}
