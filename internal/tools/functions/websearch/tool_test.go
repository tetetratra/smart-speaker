package websearch

import (
	"context"
	"strings"
	"testing"
)

type fakeSearchClient struct {
	query string
	resp  string
	err   error
}

func (f *fakeSearchClient) Search(_ context.Context, query string) (string, error) {
	f.query = query
	return f.resp, f.err
}

func TestDefinitionAcceptsOnlyQuery(t *testing.T) {
	tool := New(Config{Client: &fakeSearchClient{}})
	def := tool.Definition()
	params, ok := def["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("parameters = %#v", def["parameters"])
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties = %#v", params["properties"])
	}
	if _, ok := props["query"]; !ok {
		t.Fatal("query property should be present")
	}
	if len(props) != 1 {
		t.Fatalf("properties = %#v, want query only", props)
	}
	if got := params["additionalProperties"]; got != false {
		t.Fatalf("additionalProperties = %#v, want false", got)
	}
}

func TestRunReturnsResultOnly(t *testing.T) {
	client := &fakeSearchClient{resp: "検索結果です"}
	tool := New(Config{Client: client})
	out, err := tool.Run(map[string]any{"query": "  OpenAI web search  "})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if client.query != "OpenAI web search" {
		t.Fatalf("query = %q", client.query)
	}
	if out["result"] != "検索結果です" {
		t.Fatalf("result = %#v", out["result"])
	}
	if len(out) != 1 {
		t.Fatalf("out = %#v, want result only", out)
	}
}

func TestRunRejectsEmptyQuery(t *testing.T) {
	tool := New(Config{Client: &fakeSearchClient{}})
	_, err := tool.Run(map[string]any{"query": "   "})
	if err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Fatalf("err = %v, want query is required", err)
	}
}

func TestRunRejectsUnsupportedArgument(t *testing.T) {
	tool := New(Config{Client: &fakeSearchClient{}})
	_, err := tool.Run(map[string]any{"query": "OpenAI", "context": "不要"})
	if err == nil || !strings.Contains(err.Error(), "unsupported argument: context") {
		t.Fatalf("err = %v, want unsupported argument", err)
	}
}
