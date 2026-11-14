package claudecode

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/agentbridge"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func newTools(t *testing.T) (map[string]*Tool, *agentbridge.Service, *[]string) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		switch {
		case r.URL.Path == "/tasks":
			// 送られた作業ディレクトリの名前を返し、ツールが渡せているか確かめる。空なら既定の候補を使ったことにする。
			var body struct {
				Workdir string `json:"workdir"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Workdir == "" {
				body.Workdir = "work"
			}
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]string{"task_id": "t1", "session_id": "s1", "workdir": body.Workdir})
		case r.URL.Path == "/sessions":
			_, _ = w.Write([]byte(`{"workdirs":["work","speaker"],"sessions":[{"session_id":"s1","workdir":"speaker","title":"PDFの要約","created_at":"2026-11-07T08:00:00.000Z","last_modified":"2026-11-07T09:30:00.000Z","first_prompt":"PDFを読んで","running":false,"from_smart_speaker":true}]}`))
		case strings.HasPrefix(r.URL.Path, "/sessions/"):
			_, _ = w.Write([]byte(`{"session_id":"s1","workdir":"speaker","total":5,"offset":0,"messages":[{"role":"assistant","text":"読みました","tools":["[Read] a.pdf"],"at":"2026-11-07T09:30:00.000Z"}]}`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(server.Close)
	service := agentbridge.NewService(agentbridge.NewClient(server.URL, "token"), func(context.Context, types.ConversationInput) error { return nil })
	byName := map[string]*Tool{}
	for _, op := range Operations {
		byName[op] = &Tool{Service: service, Operation: op}
	}
	return byName, service, &requests
}

func TestAskReturnsImmediatelyWithIDs(t *testing.T) {
	tools, _, requests := newTools(t)
	result, err := tools[OpAsk].Run(context.Background(), map[string]any{"input": "PDFを読んで", "session_id": "", "workdir": ""})
	if err != nil || result["status"] != "started" || result["task_id"] != "t1" || result["session_id"] != "s1" || result["workdir"] != "work" || !strings.Contains(result["instructions"].(string), "結果を予想して話さない") {
		t.Fatal(result, err)
	}
	result, err = tools[OpAsk].Run(context.Background(), map[string]any{"input": "PDFを読んで", "session_id": "", "workdir": "speaker"})
	if err != nil || result["workdir"] != "speaker" {
		t.Fatal(result, err)
	}
	if _, err := tools[OpAsk].Run(context.Background(), map[string]any{"input": " ", "session_id": ""}); err == nil {
		t.Fatal("空の依頼を受け付けました")
	}
	if (*requests)[0] != "POST /tasks" {
		t.Fatal(*requests)
	}
}

func TestListAndReadFormatForVoice(t *testing.T) {
	time.Local = time.FixedZone("JST", 9*60*60)
	tools, _, requests := newTools(t)
	list, err := tools[OpList].Run(context.Background(), map[string]any{"limit": float64(3)})
	if err != nil {
		t.Fatal(err)
	}
	session := list["sessions"].([]map[string]any)[0]
	if workdirs := list["workdirs"].([]string); len(workdirs) != 2 || workdirs[0] != "work" || session["workdir"] != "speaker" {
		t.Fatal(list)
	}
	if session["title"] != "PDFの要約" || session["last_modified"] != "2026-11-07 18:30" || session["created_at"] != "2026-11-07 17:00" || list["count"] != 1 {
		t.Fatal(list)
	}
	read, err := tools[OpRead].Run(context.Background(), map[string]any{"session_id": "s1", "limit": float64(2), "offset": float64(0)})
	if err != nil || read["workdir"] != "speaker" || read["has_older"] != true || read["messages"].([]map[string]any)[0]["at"] != "2026-11-07 18:30" {
		t.Fatal(read, err)
	}
	if _, err := tools[OpList].Run(context.Background(), map[string]any{"limit": float64(100)}); err == nil {
		t.Fatal("範囲外の件数を受け付けました")
	}
	if _, err := tools[OpRead].Run(context.Background(), map[string]any{"session_id": "s1", "limit": 1.5}); err == nil {
		t.Fatal("小数を受け付けました")
	}
	want := []string{"GET /sessions?limit=3", "GET /sessions/s1/messages?limit=2&offset=0"}
	if strings.Join(*requests, ",") != strings.Join(want, ",") {
		t.Fatal(*requests)
	}
}

func TestAnswerAndCancel(t *testing.T) {
	tools, service, requests := newTools(t)
	event := agentbridge.Event{Type: agentbridge.EventPermissionRequest, EventID: "e1", TaskID: "t1", SessionID: "s1", RequestID: "r1", Tool: "Bash", Input: json.RawMessage(`{}`), ExpiresAt: time.Now().Add(time.Minute)}
	if err := service.HandleEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if _, err := tools[OpAnswer].Run(context.Background(), map[string]any{"request_id": "r1", "allow": "yes"}); err == nil {
		t.Fatal("文字列のallowを受け付けました")
	}
	result, err := tools[OpAnswer].Run(context.Background(), map[string]any{"request_id": "r1", "allow": true})
	if err != nil || result["status"] != "answered" {
		t.Fatal(result, err)
	}
	if _, err := tools[OpCancel].Run(context.Background(), map[string]any{"task_id": "t1", "session_id": ""}); err != nil {
		t.Fatal(err)
	}
	if _, err := tools[OpCancel].Run(context.Background(), map[string]any{"task_id": "", "session_id": "s1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := tools[OpCancel].Run(context.Background(), map[string]any{"task_id": "", "session_id": ""}); err == nil {
		t.Fatal("止める対象がないのに受け付けました")
	}
	want := []string{"POST /permissions/r1", "POST /tasks/t1/cancel", "POST /sessions/s1/cancel"}
	if strings.Join(*requests, ",") != strings.Join(want, ",") {
		t.Fatal(*requests)
	}
}

func TestDefinitionsDeclareParameters(t *testing.T) {
	for _, op := range Operations {
		def := (&Tool{Operation: op}).Definition()
		params := def["parameters"].(map[string]any)
		if def["name"] != op || len(params["required"].([]string)) != len(params["properties"].(map[string]any)) {
			t.Fatal(def)
		}
	}
	if _, err := (&Tool{Operation: OpList}).Run(context.Background(), map[string]any{}); err == nil {
		t.Fatal("設定がないのに動きました")
	}
}
