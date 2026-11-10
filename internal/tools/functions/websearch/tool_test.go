package websearch

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/types"
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

type blockingSearchClient struct {
	release chan struct{}
	resp    string
	err     error
}

func (b *blockingSearchClient) Search(ctx context.Context, _ string) (string, error) {
	select {
	case <-b.release:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return b.resp, b.err
}

// 届け先があれば、検索を待たずに受け付けを返し、終わった結果を会話へ届ける。
func TestRunAcceptsAndDeliversResultLater(t *testing.T) {
	for _, failure := range []bool{false, true} {
		client := &blockingSearchClient{release: make(chan struct{}), resp: "晴れ"}
		heading := "[Web検索の結果]"
		if failure {
			client.err = errors.New("検索APIの失敗")
			heading = "[Web検索の失敗]"
		}
		delivered := make(chan types.ScheduledPrompt, 1)
		tool := New(Config{Client: client, Deliver: func(_ context.Context, prompt types.ScheduledPrompt) error {
			delivered <- prompt
			return nil
		}})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		tool.SetContext(ctx)
		out, err := tool.Run(map[string]any{"query": "明日の天気"})
		if err != nil || out["status"] != "started" || out["query"] != "明日の天気" || out["instructions"] == "" {
			t.Fatal(out, err)
		}
		close(client.release)
		select {
		case prompt := <-delivered:
			if prompt.Source != "tool/web_search" || !strings.HasPrefix(prompt.Text, heading) || !strings.Contains(prompt.Text, "明日の天気") {
				t.Fatal(prompt)
			}
		case <-ctx.Done():
			t.Fatal("結果が届きません")
		}
		cancel()
	}
}

// アプリの停止で検索が打ち切られたら、何も届けない。
func TestCanceledSearchDeliversNothing(t *testing.T) {
	client := &blockingSearchClient{release: make(chan struct{})}
	delivered := make(chan types.ScheduledPrompt, 1)
	tool := New(Config{Client: client, Deliver: func(_ context.Context, prompt types.ScheduledPrompt) error {
		delivered <- prompt
		return nil
	}})
	ctx, cancel := context.WithCancel(context.Background())
	tool.SetContext(ctx)
	if _, err := tool.Run(map[string]any{"query": "明日の天気"}); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case prompt := <-delivered:
		t.Fatal("停止後に結果が届きました", prompt)
	case <-time.After(50 * time.Millisecond):
	}
}
