package switchbot

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tetetratra/smart-speaker/internal/tools"
)

const (
	hub2Alias                    = "hub2"
	hub2GetTemperatureToolName   = "hub2_get_temperature"
	hub2GetHumidityToolName      = "hub2_get_humidity"
	hub2GetLightLevelToolName    = "hub2_get_light_level"
)

var errNotConfigured = errors.New("SwitchBot が設定されていません")

type hub2MeasurementSpec struct {
	name        string
	description string
	bodyKey     string
	resultKey   string
}

var hub2MeasurementSpecs = []hub2MeasurementSpec{
	{
		name:        hub2GetTemperatureToolName,
		description: "Hub2の温度を取得します。",
		bodyKey:     "temperature",
		resultKey:   "temperature",
	},
	{
		name:        hub2GetHumidityToolName,
		description: "Hub2の湿度を取得します。",
		bodyKey:     "humidity",
		resultKey:   "humidity",
	},
	{
		name:        hub2GetLightLevelToolName,
		description: "Hub2の照度を取得します。",
		bodyKey:     "lightLevel",
		resultKey:   "light_level",
	},
}

// hub2MeasurementTool は Hub2 から単一の環境値を取得するツールです。
type hub2MeasurementTool struct {
	spec   hub2MeasurementSpec
	client *Client
	ctx    context.Context
}

// NewHub2ToolsWithClient は温度・湿度・照度の Hub2 ツール群を生成します。
func NewHub2ToolsWithClient(client *Client) []*hub2MeasurementTool {
	if client == nil {
		return nil
	}
	out := make([]*hub2MeasurementTool, 0, len(hub2MeasurementSpecs))
	for _, spec := range hub2MeasurementSpecs {
		out = append(out, &hub2MeasurementTool{spec: spec, client: client})
	}
	return out
}

func (t *hub2MeasurementTool) Name() string { return t.spec.name }

func (t *hub2MeasurementTool) SetContext(ctx context.Context) {
	t.ctx = ctx
}

func (t *hub2MeasurementTool) Run(args map[string]any) (map[string]any, error) {
	if t.client == nil {
		return nil, errNotConfigured
	}
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	status, err := t.client.GetStatus(ctx, "", hub2Alias)
	if err != nil {
		return nil, err
	}
	body, _ := status["body"].(map[string]any)
	value := "取得不可"
	if body != nil {
		if v, ok := body[t.spec.bodyKey]; ok {
			value = normalizeValue(v)
		}
	}
	return map[string]any{t.spec.resultKey: value}, nil
}

func (t *hub2MeasurementTool) Definition() map[string]any {
	return tools.DefinitionWithMode(map[string]any{
		"type":        "function",
		"name":        t.spec.name,
		"description": t.spec.description,
		"parameters": map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
	}, tools.ToolModeRead)
}

var _ tools.Handler = (*hub2MeasurementTool)(nil)
var _ tools.ContextAware = (*hub2MeasurementTool)(nil)
var _ tools.DefinitionProvider = (*hub2MeasurementTool)(nil)

func normalizeValue(v any) string {
	text := strings.TrimSpace(fmt.Sprint(v))
	if text == "" || text == "<nil>" {
		return "取得不可"
	}
	return text
}
