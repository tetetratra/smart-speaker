package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tetetratra/smart-speaker/internal/tools"
)

const (
	// maxToolNameLength は Realtime API の function 名の長さの上限。
	maxToolNameLength = 64
	// maxResultLength は会話モデルへ返す結果の文字数の上限。長い結果で会話の文脈を使い切らないようにする。
	maxResultLength = 20000
)

var invalidToolNameChars = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// tool は MCP サーバーのツール1つを AI ツールにしたもの。
type tool struct {
	server *server
	// name は会話モデルに見せる名前（<サーバー名>__<ツール名>）。
	name string
	// remoteName は MCP サーバー上のツール名。
	remoteName  string
	description string
	parameters  map[string]any
}

func newTool(s *server, t *sdk.Tool) (*tool, error) {
	toolName := s.name + "__" + invalidToolNameChars.ReplaceAllString(t.Name, "_")
	if len(toolName) > maxToolNameLength {
		return nil, fmt.Errorf("ツール名 %s が %d 文字を超えます", toolName, maxToolNameLength)
	}
	parameters, err := toolParameters(t.InputSchema)
	if err != nil {
		return nil, err
	}
	description := strings.TrimSpace(t.Description)
	if t.Title != "" {
		description = strings.TrimSpace(t.Title + "\n" + description)
	}
	return &tool{server: s, name: toolName, remoteName: t.Name, description: description, parameters: parameters}, nil
}

// toolParameters は MCP の inputSchema を function ツールの parameters にする。
func toolParameters(inputSchema any) (map[string]any, error) {
	parameters := map[string]any{}
	if inputSchema != nil {
		data, err := json.Marshal(inputSchema)
		if err != nil {
			return nil, fmt.Errorf("inputSchema を読めません: %w", err)
		}
		if err := json.Unmarshal(data, &parameters); err != nil {
			return nil, fmt.Errorf("inputSchema がオブジェクトではありません: %w", err)
		}
	}
	delete(parameters, "$schema")
	if _, ok := parameters["type"]; !ok {
		parameters["type"] = "object"
	}
	if _, ok := parameters["properties"]; !ok {
		parameters["properties"] = map[string]any{}
	}
	return parameters, nil
}

func (t *tool) Name() string { return t.name }

func (t *tool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        t.name,
		"description": t.description,
		"parameters":  t.parameters,
	}
}

// Run は MCP の tools/call を呼び、結果のテキストを返す。ツールが失敗を返したら、その内容をエラーにする。
func (t *tool) Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	result, err := t.server.callTool(ctx, t.remoteName, args)
	if err != nil {
		var authErr *authRequiredError
		if errors.As(err, &authErr) {
			return nil, err
		}
		return nil, fmt.Errorf("MCPサーバー %s のツール %s を呼べませんでした: %w", t.server.name, t.remoteName, err)
	}
	text := resultText(result)
	if result.IsError {
		return nil, fmt.Errorf("MCPサーバー %s のツール %s が失敗しました: %s", t.server.name, t.remoteName, text)
	}
	return map[string]any{"result": text}, nil
}

// resultText は結果の内容をテキストにまとめる。テキストがなく構造化された結果だけあれば、それを JSON にする。
func resultText(result *sdk.CallToolResult) string {
	var parts []string
	for _, content := range result.Content {
		switch c := content.(type) {
		case *sdk.TextContent:
			parts = append(parts, c.Text)
		case *sdk.ImageContent:
			parts = append(parts, "（画像: "+c.MIMEType+"）")
		case *sdk.AudioContent:
			parts = append(parts, "（音声: "+c.MIMEType+"）")
		case *sdk.ResourceLink:
			parts = append(parts, "（リソース: "+c.URI+"）")
		case *sdk.EmbeddedResource:
			if c.Resource != nil && c.Resource.Text != "" {
				parts = append(parts, c.Resource.Text)
			} else if c.Resource != nil {
				parts = append(parts, "（リソース: "+c.Resource.URI+"）")
			}
		}
	}
	text := strings.Join(parts, "\n")
	if text == "" && result.StructuredContent != nil {
		if data, err := json.Marshal(result.StructuredContent); err == nil {
			text = string(data)
		}
	}
	if runes := []rune(text); len(runes) > maxResultLength {
		text = string(runes[:maxResultLength]) + "\n（長いため、ここで切り詰めました）"
	}
	return text
}

var _ tools.Handler = (*tool)(nil)
var _ tools.DefinitionProvider = (*tool)(nil)
