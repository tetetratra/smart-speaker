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

// Client knows how to talk to the SwitchBot Open API.
type Client struct {
	token     string
	secret    string
	http      *http.Client
	deviceMap map[string]string
}

// Command declares the payload expected from the function call arguments.
type Command struct {
	DeviceAlias string
	DeviceID    string
	Command     string
	CommandType string
	Parameter   string
}

// NewFromEnv builds a client using environment variables.
func NewFromEnv() (*Client, error) {
	token := strings.TrimSpace(os.Getenv("SWITCHBOT_TOKEN"))
	secret := strings.TrimSpace(os.Getenv("SWITCHBOT_SECRET"))
	if token == "" || secret == "" {
		return nil, errors.New("SWITCHBOT_TOKEN と SWITCHBOT_SECRET を設定してください")
	}

	deviceMap, err := parseDeviceMap(os.Getenv("SWITCHBOT_DEVICE_MAP"))
	if err != nil {
		return nil, err
	}

	client := &Client{
		token:     token,
		secret:    secret,
		http:      &http.Client{Timeout: 10 * time.Second},
		deviceMap: deviceMap,
	}
	return client, nil
}

// Execute sends a command to the SwitchBot API and returns the raw response body as a map.
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

func parseDeviceMap(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}, nil
	}
	var tmp map[string]string
	if err := json.Unmarshal([]byte(raw), &tmp); err != nil {
		return nil, fmt.Errorf("SWITCHBOT_DEVICE_MAP は JSON オブジェクトで指定してください: %w", err)
	}
	normalized := make(map[string]string, len(tmp))
	for k, v := range tmp {
		normalized[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return normalized, nil
}
