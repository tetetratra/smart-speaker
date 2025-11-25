package toolcaller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WebFetchTool retrieves the contents of a URL via HTTP GET.
type WebFetchTool struct {
	client  *http.Client
	ctx     context.Context
	timeout time.Duration
}

// NewWebFetchTool constructs a fetcher with sane defaults.
func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{
		client:  &http.Client{},
		timeout: 10 * time.Second,
	}
}

func (t *WebFetchTool) Name() string {
	return "web_fetch"
}

// SetContext allows the tool to respect the stage lifecycle.
func (t *WebFetchTool) SetContext(ctx context.Context) {
	t.ctx = ctx
}

func (t *WebFetchTool) Run(args map[string]any) (map[string]any, error) {
	rawURL, ok := args["url"]
	if !ok {
		return nil, fmt.Errorf("url argument is required")
	}
	urlStr, ok := rawURL.(string)
	if !ok {
		return nil, fmt.Errorf("url must be a string")
	}
	urlStr = strings.TrimSpace(urlStr)
	if urlStr == "" {
		return nil, fmt.Errorf("url cannot be empty")
	}

	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if t.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	reader := io.LimitReader(resp.Body, 512*1024)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"status": resp.StatusCode,
		"body":   string(body),
	}, nil
}
