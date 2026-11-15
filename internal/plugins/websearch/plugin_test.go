package websearch

import (
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/plugin"
)

// OpenAI のキーとモデルがそろったときだけ、ツールと使い方の指示文を出す。
func TestPluginNeedsOpenAIConfig(t *testing.T) {
	for _, cfg := range []plugin.OpenAIConfig{{}, {APIKey: "key"}, {ResponsesModel: "model"}} {
		p := New(plugin.Deps{OpenAI: cfg})
		if len(p.Tools()) != 0 || p.Instructions() != "" {
			t.Fatalf("設定が足りないのにツールか指示文が出ています: %+v", cfg)
		}
	}
	p := New(plugin.Deps{OpenAI: plugin.OpenAIConfig{APIKey: "key", ResponsesModel: "model"}})
	tools := p.Tools()
	if len(tools) != 1 || tools[0].Name() != "web_search" {
		t.Fatalf("ツール = %#v", tools)
	}
	def := tools[0].(*Tool).Definition()
	props := def["parameters"].(map[string]any)["properties"].(map[string]any)
	if _, ok := props["query"]; !ok || len(props) != 1 {
		t.Fatalf("properties = %#v、query だけのはず", props)
	}
	if !strings.Contains(p.Instructions(), "web_search") {
		t.Fatalf("指示文 = %q", p.Instructions())
	}
}
