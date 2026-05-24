package websearch

import (
	"context"
	"fmt"
	"strings"

	"github.com/tetetratra/smart-speaker/internal/tools"
)

const (
	toolName = "web_search"

	toolDescription = `Web検索が必要な最新情報・外部情報を調べます。
検索したい内容をqueryに簡潔に書いてください。
`
	queryDescription = `検索したい内容。`
)

type SearchClient interface {
	Search(ctx context.Context, query string) (string, error)
}

type Config struct {
	APIKey string
	Model  string
	Client SearchClient
}

type Tool struct {
	client SearchClient
	ctx    context.Context
}

func New(cfg Config) *Tool {
	client := cfg.Client
	if client == nil {
		client = NewClient(ClientConfig{
			APIKey: cfg.APIKey,
			Model:  cfg.Model,
		})
	}
	return &Tool{client: client}
}

func (t *Tool) Name() string { return toolName }

func (t *Tool) SetContext(ctx context.Context) {
	t.ctx = ctx
}

func (t *Tool) Run(args map[string]any) (map[string]any, error) {
	if t.client == nil {
		return nil, fmt.Errorf("web_search client is not configured")
	}
	if args == nil {
		args = map[string]any{}
	}
	for name := range args {
		if name != "query" {
			return nil, fmt.Errorf("unsupported argument: %s", name)
		}
	}
	query := strings.TrimSpace(asString(args["query"]))
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	result, err := t.client.Search(t.ctxOrBackground(), query)
	if err != nil {
		return nil, err
	}
	return map[string]any{"result": result}, nil
}

func (t *Tool) Definition() map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        toolName,
		"description": toolDescription,
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": queryDescription,
				},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
	}
}

func (t *Tool) ctxOrBackground() context.Context {
	if t.ctx != nil {
		return t.ctx
	}
	return context.Background()
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

var _ tools.Handler = (*Tool)(nil)
var _ tools.ContextAware = (*Tool)(nil)
var _ tools.DefinitionProvider = (*Tool)(nil)
