package switchbot

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const baseURL = "https://api.switch-bot.com"

// API仕様書: https://github.com/OpenWonderLabs/SwitchBotAPI

// SwitchBot Open API との通信を担当する
type Client struct {
	token     string
	secret    string
	http      *http.Client
	baseURL   string
	clockSkew atomic.Int64
}

// Device はデバイス一覧から監視対象を選ぶための情報。
type Device struct {
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	DeviceType string `json:"deviceType"`
}

type Scene struct {
	SceneID   string
	SceneName string
}

type apiResponse[T any] struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
	Body       T      `json:"body"`
}

// NDJSON tool item から受け取るパラメータの構造
type Command struct {
	DeviceID    string
	Command     string
	CommandType string
	Parameter   string
}

// 設定値に基づいてクライアントを初期化する
func NewSwitchbotClient(token, secret string) *Client {
	token = strings.TrimSpace(token)
	secret = strings.TrimSpace(secret)
	if token == "" || secret == "" {
		panic("SwitchBot token and secret must be provided")
	}

	client := &Client{
		token:   token,
		secret:  secret,
		http:    &http.Client{Timeout: 10 * time.Second},
		baseURL: baseURL,
	}
	return client
}

// SwitchBot API にコマンドを送りレスポンスを返す
func (c *Client) Execute(ctx context.Context, cmd Command) (map[string]any, error) {
	deviceID := strings.TrimSpace(cmd.DeviceID)
	if deviceID == "" {
		return nil, errors.New("device_id を指定してください")
	}

	command := strings.TrimSpace(cmd.Command)
	if command == "" {
		return nil, errors.New("command を指定してください")
	}

	commandType := strings.TrimSpace(cmd.CommandType)
	if commandType == "" {
		commandType = "command"
	}
	parameter := strings.TrimSpace(cmd.Parameter)
	if parameter == "" {
		parameter = "default"
	}

	payload := map[string]string{
		"command":     command,
		"commandType": commandType,
		"parameter":   parameter,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v1.1/devices/%s/commands", c.baseURL, deviceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
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

	out, err := decodeAPIResponse[map[string]any](resp)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"statusCode":  out.StatusCode,
		"message":     out.Message,
		"body":        out.Body,
		"http_status": resp.StatusCode,
		"device_id":   deviceID,
	}, nil
}

// GetStatus はデバイスのステータスを取得します。
func (c *Client) GetStatus(ctx context.Context, deviceID string) (map[string]any, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil, errors.New("device_id を指定してください")
	}
	url := fmt.Sprintf("%s/v1.1/devices/%s/status", c.baseURL, deviceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

	out, err := decodeAPIResponse[map[string]any](resp)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"statusCode":  out.StatusCode,
		"message":     out.Message,
		"body":        out.Body,
		"http_status": resp.StatusCode,
		"device_id":   deviceID,
	}, nil
}

// ListDevices はアカウントに登録された物理デバイスを API の並び順で返す。
func (c *Client) ListDevices(ctx context.Context) ([]Device, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1.1/devices", nil)
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
	out, err := decodeAPIResponse[struct {
		DeviceList []Device `json:"deviceList"`
	}](resp)
	if err != nil {
		return nil, err
	}
	return out.Body.DeviceList, nil
}

func (c *Client) ListScenes(ctx context.Context) ([]Scene, error) {
	url := fmt.Sprintf("%s/v1.1/scenes", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

	out, err := decodeAPIResponse[[]struct {
		SceneID   string `json:"sceneId"`
		SceneName string `json:"sceneName"`
	}](resp)
	if err != nil {
		return nil, err
	}

	scenes := make([]Scene, 0, len(out.Body))
	for _, item := range out.Body {
		sceneID := strings.TrimSpace(item.SceneID)
		sceneName := strings.TrimSpace(item.SceneName)
		if sceneID == "" || sceneName == "" {
			continue
		}
		scenes = append(scenes, Scene{SceneID: sceneID, SceneName: sceneName})
	}
	return scenes, nil
}

func (c *Client) ExecuteScene(ctx context.Context, sceneID string) (map[string]any, error) {
	sceneID = strings.TrimSpace(sceneID)
	if sceneID == "" {
		return nil, errors.New("scene_id を指定してください")
	}

	url := fmt.Sprintf("%s/v1.1/scenes/%s/execute", c.baseURL, sceneID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
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

	out, err := decodeAPIResponse[map[string]any](resp)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"statusCode":  out.StatusCode,
		"message":     out.Message,
		"body":        out.Body,
		"http_status": resp.StatusCode,
		"scene_id":    sceneID,
	}, nil
}

func (c *Client) signPayload(timestamp, nonce string) (string, error) {
	mac := hmac.New(sha256.New, []byte(c.secret))
	if _, err := mac.Write([]byte(c.token + timestamp + nonce)); err != nil {
		return "", err
	}
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return sig, nil
}

func (c *Client) applyAuthHeaders(req *http.Request, timestamp, nonce, signature string) {
	req.Header.Set("Content-Type", "application/json; charset=utf8")
	req.Header.Set("Authorization", c.token)
	req.Header.Set("t", timestamp)
	req.Header.Set("nonce", nonce)
	req.Header.Set("sign", signature)
}

func (c *Client) applyAuth(req *http.Request) error {
	nonce := uuid.NewString()
	timestamp := strconv.FormatInt(c.currentTime().UnixMilli(), 10)
	signature, err := c.signPayload(timestamp, nonce)
	if err != nil {
		return err
	}
	c.applyAuthHeaders(req, timestamp, nonce, signature)
	return nil
}

func (c *Client) currentTime() time.Time {
	return time.Now().Add(time.Duration(c.clockSkew.Load()))
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if !shouldRetrySwitchBotAuth(resp) {
		return resp, nil
	}
	serverTime, ok := parseHTTPDate(resp.Header.Get("Date"))
	if !ok {
		return resp, nil
	}

	c.clockSkew.Store(serverTime.Sub(time.Now()).Nanoseconds())
	resp.Body.Close()

	retryReq := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		retryReq.Body = body
	}
	if err := c.applyAuth(retryReq); err != nil {
		return nil, err
	}
	return c.http.Do(retryReq)
}

func shouldRetrySwitchBotAuth(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	return resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden
}

func parseHTTPDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(http.TimeFormat, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func decodeAPIResponse[T any](resp *http.Response) (apiResponse[T], error) {
	var out apiResponse[T]
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	diagnosticCategory := switchBotDiagnosticCategory(resp.StatusCode, out.StatusCode, out.Message)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return out, fmt.Errorf(
			"SwitchBot API request failed: http_status=%d status_code=%d message=%q request_id=%q diagnostic_category=%q",
			resp.StatusCode,
			out.StatusCode,
			out.Message,
			resp.Header.Get("switchbot-request-id"),
			diagnosticCategory,
		)
	}
	if out.StatusCode != 100 {
		return out, fmt.Errorf(
			"SwitchBot API returned failure: http_status=%d status_code=%d message=%q request_id=%q diagnostic_category=%q",
			resp.StatusCode,
			out.StatusCode,
			out.Message,
			resp.Header.Get("switchbot-request-id"),
			diagnosticCategory,
		)
	}
	return out, nil
}

func switchBotDiagnosticCategory(httpStatus, statusCode int, message string) string {
	normalized := strings.ToLower(message)
	if httpStatus == http.StatusTooManyRequests || strings.Contains(normalized, "rate") || strings.Contains(normalized, "too many") {
		return "rate_limited"
	}
	if strings.Contains(normalized, "sign") ||
		strings.Contains(normalized, "signature") ||
		strings.Contains(normalized, "timestamp") ||
		strings.Contains(normalized, "nonce") ||
		strings.Contains(normalized, "clock") ||
		strings.Contains(normalized, "time") {
		return "clock_skew_or_signature"
	}
	if httpStatus == http.StatusUnauthorized ||
		httpStatus == http.StatusForbidden ||
		strings.Contains(normalized, "unauthorized") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "auth") {
		return "auth_failed"
	}
	if statusCode == 190 && strings.Contains(normalized, "request limit") {
		return "rate_limited"
	}
	return "api_failure"
}
