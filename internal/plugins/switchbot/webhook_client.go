package switchbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// WebhookURLs は現在の送信先を照会する。登録コマンドはこれを先に呼び、別の送信先を上書きしない。
func (c *Client) WebhookURLs(ctx context.Context) ([]string, error) {
	body, err := c.webhookRequest(ctx, "queryWebhook", map[string]any{"action": "queryUrl"})
	if err != nil {
		return nil, err
	}
	var out struct {
		URLs []string `json:"urls"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.URLs == nil {
		return nil, errors.New("Webhook照会の応答にurlsがありません")
	}
	return out.URLs, nil
}

func (c *Client) RegisterWebhook(ctx context.Context, callbackURL string) error {
	_, err := c.webhookRequest(ctx, "setupWebhook", map[string]any{"action": "setupWebhook", "url": callbackURL, "deviceList": "ALL"})
	return err
}

func (c *Client) DeleteWebhook(ctx context.Context, callbackURL string) error {
	_, err := c.webhookRequest(ctx, "deleteWebhook", map[string]any{"action": "deleteWebhook", "url": callbackURL})
	return err
}

func (c *Client) webhookRequest(ctx context.Context, endpoint string, payload any) (json.RawMessage, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1.1/webhook/"+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if err := c.applyAuth(req); err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out, err := decodeAPIResponse[json.RawMessage](resp)
	return out.Body, err
}
