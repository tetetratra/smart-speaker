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
	deviceMap map[string]string
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

// function calling から受け取るパラメータの構造
type Command struct {
	DeviceAlias string
	DeviceID    string
	Command     string
	CommandType string
	Parameter   string
}

// 設定値に基づいてクライアントを初期化する
func NewSwitchbotClient(token, secret, deviceMapRaw string) *Client {
	token = strings.TrimSpace(token)
	secret = strings.TrimSpace(secret)
	if token == "" || secret == "" {
		panic("SwitchBot token and secret must be provided")
	}

	deviceMap := parseDeviceMap(deviceMapRaw)

	client := &Client{
		token:     token,
		secret:    secret,
		http:      &http.Client{Timeout: 10 * time.Second},
		baseURL:   baseURL,
		deviceMap: deviceMap,
	}
	return client
}

// SwitchBot API にコマンドを送りレスポンスを返す
func (c *Client) Execute(ctx context.Context, cmd Command) (map[string]any, error) {
	deviceID, err := c.resolveDeviceID(cmd.DeviceID, cmd.DeviceAlias)
	if err != nil {
		return nil, err
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

	nonce := uuid.NewString()
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	signature, err := c.signPayload(timestamp, nonce)
	if err != nil {
		return nil, err
	}

	c.applyAuthHeaders(req, timestamp, nonce, signature)

	resp, err := c.http.Do(req)
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
func (c *Client) GetStatus(ctx context.Context, deviceID, deviceAlias string) (map[string]any, error) {
	resolvedID, err := c.resolveDeviceID(deviceID, deviceAlias)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/v1.1/devices/%s/status", c.baseURL, resolvedID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	nonce := uuid.NewString()
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	signature, err := c.signPayload(timestamp, nonce)
	if err != nil {
		return nil, err
	}
	c.applyAuthHeaders(req, timestamp, nonce, signature)

	resp, err := c.http.Do(req)
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
		"device_id":   resolvedID,
	}, nil
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

	resp, err := c.http.Do(req)
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

	resp, err := c.http.Do(req)
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

func (c *Client) resolveDeviceID(deviceID, deviceAlias string) (string, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" && deviceAlias != "" {
		var ok bool
		deviceID, ok = c.deviceMap[strings.ToLower(deviceAlias)]
		if !ok {
			return "", fmt.Errorf("未定義のデバイス名です: %s", deviceAlias)
		}
	}
	if deviceID == "" {
		return "", errors.New("device_id か device (エイリアス) のいずれかを指定してください")
	}
	return deviceID, nil
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
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	signature, err := c.signPayload(timestamp, nonce)
	if err != nil {
		return err
	}
	c.applyAuthHeaders(req, timestamp, nonce, signature)
	return nil
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
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return out, fmt.Errorf(
			"SwitchBot API request failed: http_status=%d status_code=%d message=%q request_id=%q",
			resp.StatusCode,
			out.StatusCode,
			out.Message,
			resp.Header.Get("switchbot-request-id"),
		)
	}
	if out.StatusCode != 100 {
		return out, fmt.Errorf(
			"SwitchBot API returned failure: http_status=%d status_code=%d message=%q request_id=%q",
			resp.StatusCode,
			out.StatusCode,
			out.Message,
			resp.Header.Get("switchbot-request-id"),
		)
	}
	return out, nil
}

func parseDeviceMap(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}
	}
	var tmp map[string]string
	if err := json.Unmarshal([]byte(raw), &tmp); err != nil {
		panic("SWITCHBOT_DEVICE_MAP は JSON オブジェクトで指定してください")
	}
	normalized := make(map[string]string, len(tmp))
	for k, v := range tmp {
		normalized[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return normalized
}
