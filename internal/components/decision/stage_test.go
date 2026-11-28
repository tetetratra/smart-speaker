package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func testStage(t *testing.T, handler http.HandlerFunc) *stage {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	history, err := conversationhistory.NewStore(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := history.Append(types.ConversationRecord{ID: "1", Role: types.RoleUser, Text: "明日の天気は？", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	return &stage{cfg: Config{APIKey: "local-test", Model: "gpt-6-luna", Endpoint: server.URL, History: history, Client: server.Client()}}
}

// 会話の履歴と今回の発話を Decisions API に渡し、選ばれた選択肢を返す。
func TestDecideSendsHistoryAndReturnsChoice(t *testing.T) {
	var payload map[string]any
	s := testStage(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-test" {
			t.Error(r.Header)
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		_, _ = w.Write([]byte(`{"answers":[{"type":"choice","name":"turn","choice":"delegate_light","confidence":0.9,"probabilities":[]}],"usage":{"input_tokens":10}}`))
	})
	got := s.decide(context.Background())
	if got != types.DecisionDelegateLight {
		t.Fatal(got)
	}
	if payload["model"] != "gpt-6-luna" || !strings.Contains(payload["input"].(string), "明日の天気は？") {
		t.Fatal(payload)
	}
	question := payload["questions"].([]any)[0].(map[string]any)
	var values []string
	for _, c := range question["choices"].([]any) {
		values = append(values, c.(map[string]any)["value"].(string))
	}
	if question["type"] != "choice" || strings.Join(values, ",") != "respond,delegate_light,delegate_strong,wait,ignore" {
		t.Fatal(question)
	}
}

// 判定に失敗したり答えを拒否されたりしたら、無言で止まらないよう強いモデルに任せる。
func TestDecideFallsBackToStrongModel(t *testing.T) {
	for _, body := range []string{`{"answers":[{"type":"refusal","name":"turn"}]}`, `{"answers":[{"type":"choice","choice":"unknown"}]}`, `broken`} {
		s := testStage(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })
		if got := s.decide(context.Background()); got != types.DecisionDelegateStrong {
			t.Fatal(body, got)
		}
	}
	s := testStage(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "overloaded", http.StatusServiceUnavailable)
	})
	if got := s.decide(context.Background()); got != types.DecisionDelegateStrong {
		t.Fatal(got)
	}
}

// 判定結果は、依頼のターンと世代を付けて返す。
func TestRunReturnsDecisionForRequest(t *testing.T) {
	s := testStage(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"answers":[{"type":"choice","choice":"respond","confidence":0.9}]}`))
	})
	s.node = &graph.Stage{Upstream: make(chan types.Event), Downstream: make(chan types.Event)}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go s.run(ctx)
	s.node.Upstream <- types.EventDecisionRequest{DecisionRequest: types.DecisionRequest{TurnID: "turn1", GenerationID: 3}}
	got := (<-s.node.Downstream).(types.EventDecision)
	if got.TurnID != "turn1" || got.GenerationID != 3 || got.Choice != types.DecisionRespond {
		t.Fatal(got)
	}
}

// 判定に迷っている（confidence が低い）ときは、強いモデルに任せる。
func TestDecideUsesStrongModelWhenConfidenceIsLow(t *testing.T) {
	for _, body := range []string{
		`{"answers":[{"type":"choice","choice":"delegate_light","confidence":0.3}]}`,
		`{"answers":[{"type":"choice","choice":"respond","confidence":0.49}]}`,
	} {
		s := testStage(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) })
		if got := s.decide(context.Background()); got != types.DecisionDelegateStrong {
			t.Fatal(body, got)
		}
	}
}
