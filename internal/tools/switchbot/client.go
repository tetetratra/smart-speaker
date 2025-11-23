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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const baseURL = "https://api.switch-bot.com"

// SwitchBot Open API との通信を担当する
type Client struct {
	token     string
	secret    string
	http      *http.Client
	deviceMap map[string]string
}

// function calling から受け取るパラメータの構造
type Command struct {
	DeviceAlias string
	DeviceID    string
	Command     string
	CommandType string
	Parameter   string
}

// 環境変数に基づいてクライアントを初期化する
func NewFromEnv() *Client {
	token := strings.TrimSpace(os.Getenv("SWITCHBOT_TOKEN"))
	secret := strings.TrimSpace(os.Getenv("SWITCHBOT_SECRET"))
	if token == "" || secret == "" {
		panic("SWITCHBOT_TOKEN と SWITCHBOT_SECRET を設定してください")
	}

	deviceMap := parseDeviceMap(os.Getenv("SWITCHBOT_DEVICE_MAP"))

	client := &Client{
		token:     token,
		secret:    secret,
		http:      &http.Client{Timeout: 10 * time.Second},
		deviceMap: deviceMap,
	}
	return client
}

// SwitchBot API にコマンドを送りレスポンスを返す
func (c *Client) Execute(ctx context.Context, cmd Command) (map[string]any, error) {
	deviceID := strings.TrimSpace(cmd.DeviceID)
	if deviceID == "" && cmd.DeviceAlias != "" {
		var ok bool
		deviceID, ok = c.deviceMap[strings.ToLower(cmd.DeviceAlias)]
		if !ok {
			return nil, fmt.Errorf("未定義のデバイス名です: %s", cmd.DeviceAlias)
		}
	}
	if deviceID == "" {
		return nil, errors.New("device_id か device (エイリアス) のいずれかを指定してください")
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

	url := fmt.Sprintf("%s/v1.1/devices/%s/commands", baseURL, deviceID)
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

	req.Header.Set("Content-Type", "application/json; charset=utf8")
	req.Header.Set("Authorization", c.token)
	req.Header.Set("t", timestamp)
	req.Header.Set("nonce", nonce)
	req.Header.Set("sign", signature)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	out["http_status"] = resp.StatusCode
	out["device_id"] = deviceID
	return out, nil
}

func (c *Client) signPayload(timestamp, nonce string) (string, error) {
	mac := hmac.New(sha256.New, []byte(c.secret))
	if _, err := mac.Write([]byte(c.token + timestamp + nonce)); err != nil {
		return "", err
	}
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return sig, nil
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
