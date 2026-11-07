package agentbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client はブリッジのHTTP APIを呼ぶ。
// ツールの実行は直列なので、Macが眠っていても他のツールを長く止めないよう、短い期限で諦める。
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext},
		},
	}
}

// Error はブリッジがエラーで答えたときの内容。
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("Claude Codeのブリッジがエラーを返しました（%d）: %s", e.Status, e.Message)
}

func (c *Client) StartTask(ctx context.Context, input, sessionID string) (TaskAccepted, error) {
	var out TaskAccepted
	err := c.do(ctx, http.MethodPost, "/tasks", map[string]string{"input": input, "session_id": sessionID}, &out)
	return out, err
}

func (c *Client) AnswerPermission(ctx context.Context, requestID string, allow bool) error {
	return c.do(ctx, http.MethodPost, "/permissions/"+url.PathEscape(requestID), map[string]any{"allow": allow}, nil)
}

func (c *Client) CancelTask(ctx context.Context, taskID string) error {
	return c.do(ctx, http.MethodPost, "/tasks/"+url.PathEscape(taskID)+"/cancel", nil, nil)
}

// CancelSession はセッションで実行中の作業を止める。task_id が分からなくなった後に使う。
func (c *Client) CancelSession(ctx context.Context, sessionID string) error {
	return c.do(ctx, http.MethodPost, "/sessions/"+url.PathEscape(sessionID)+"/cancel", nil, nil)
}

func (c *Client) ListSessions(ctx context.Context, limit int) ([]SessionSummary, error) {
	var out struct {
		Sessions []SessionSummary `json:"sessions"`
	}
	err := c.do(ctx, http.MethodGet, "/sessions?limit="+strconv.Itoa(limit), nil, &out)
	return out.Sessions, err
}

func (c *Client) ReadSession(ctx context.Context, sessionID string, limit, offset int) (SessionMessages, error) {
	var out SessionMessages
	path := fmt.Sprintf("/sessions/%s/messages?limit=%d&offset=%d", url.PathEscape(sessionID), limit, offset)
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("Claude Codeのブリッジ（Mac）に接続できません。Macがスリープ中か、ブリッジが止まっている可能性があります: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		var payload struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &payload)
		if payload.Error == "" {
			payload.Error = strings.TrimSpace(string(data))
		}
		return &Error{Status: resp.StatusCode, Message: payload.Error}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
