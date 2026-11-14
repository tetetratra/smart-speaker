package realtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/tetetratra/smart-speaker/internal/components/conversationmanager"
	"github.com/tetetratra/smart-speaker/internal/components/generationcontroller"
	"github.com/tetetratra/smart-speaker/internal/eventqueue"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	memorystate "github.com/tetetratra/smart-speaker/internal/states/memory"
	"github.com/tetetratra/smart-speaker/internal/types"
	"net/http"
	"net/http/httptest"
	"nhooyr.io/websocket"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type rig struct {
	managerQueue *eventqueue.Queue[types.Event]
	connections  chan *websocket.Conn
	t            *testing.T
	ctx          context.Context
	node         *graph.Stage
	conn         *websocket.Conn
	commands     chan map[string]any
	events       chan types.Event
	history      *conversationhistory.Store
}

func newRig(t *testing.T, options ...func(*Config)) *rig {
	return newRigWithInitialText(t, "", options...)
}
func newRigWithInbound(t *testing.T, inbound <-chan types.InboundMessage, options ...func(*Config)) *rig {
	return createRig(t, "", inbound, options...)
}
func newRigWithInitialText(t *testing.T, initialText string, options ...func(*Config)) *rig {
	return createRig(t, initialText, nil, options...)
}
func newTestHistory(t *testing.T) *conversationhistory.Store {
	t.Helper()
	history, err := conversationhistory.NewStore(filepath.Join(t.TempDir(), "conversation_history.json"))
	if err != nil {
		t.Fatal(err)
	}
	return history
}

func createRig(t *testing.T, initialText string, inbound <-chan types.InboundMessage, options ...func(*Config)) *rig {
	t.Helper()
	// 会話管理のゴルーチンは停止の直後まで保存しうるため、t.TempDirの厳密な後片付けを避ける。まれに一時ディレクトリが残っても構わない。
	dir, err := os.MkdirTemp("", "realtime-rig")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	t.Cleanup(cancel)
	history, err := conversationhistory.NewStore(filepath.Join(dir, "conversation_history.json"))
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{t: t, ctx: ctx, commands: make(chan map[string]any, 100), events: make(chan types.Event, 100), history: history}
	connected := make(chan *websocket.Conn, 1)
	r.connections = connected
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		c, err := websocket.Accept(w, req, nil)
		if err != nil {
			return
		}
		defer c.Close(websocket.StatusNormalClosure, "")
		connected <- c
		for {
			_, raw, err := c.Read(ctx)
			if err != nil {
				return
			}
			var msg map[string]any
			if json.Unmarshal(raw, &msg) != nil {
				return
			}
			select {
			case r.commands <- msg:
			case <-ctx.Done():
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	cfg := Config{APIKey: "local-test", Endpoint: strings.Replace(server.URL, "http", "ws", 1), Generation: generation.NewStore(), History: r.history}
	for _, option := range options {
		option(&cfg)
	}
	node, err := NewStage(cfg)
	if err != nil {
		t.Fatal(err)
	}
	r.node = node
	manager := conversationmanager.NewStage(conversationmanager.Config{Inbound: inbound, History: r.history})
	managerQueue := eventqueue.New[types.Event](nil)
	r.managerQueue = managerQueue
	go manager.Run(ctx)
	go func() {
		for {
			e, ok := managerQueue.Pop(ctx)
			if !ok {
				return
			}
			select {
			case manager.Upstream <- e:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-manager.Downstream:
				if !ok {
					return
				}
				select {
				case node.Upstream <- e:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	go node.Run(ctx)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-node.Downstream:
				if !ok {
					return
				}
				switch e.Kind {
				case types.EventConversationRecord:
					managerQueue.Push(e)
				case types.EventConversationResponseRequest, types.EventConversationContextAddition, types.EventConversationHistorySync, types.EventConversationIdle, types.EventConversationRecordsUsed:
					managerQueue.Push(e)
				}

				select {
				case r.events <- e:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	if initialText == "__offline__" {
		return r
	}
	if initialText == "" {
		node.Upstream <- types.Event{Payload: types.RTCPeerOutputSink{PeerID: "test", Connected: true}}
	} else {
		node.Upstream <- types.Event{Payload: types.LLMRequest{Text: initialText}}
	}
	select {
	case r.conn = <-connected:
	case <-ctx.Done():
		t.Fatal("接続できない")
	}
	update := r.wait("session.update")
	session := update["session"].(map[string]any)
	vad := session["audio"].(map[string]any)["input"].(map[string]any)["turn_detection"].(map[string]any)
	if vad["create_response"] != false || vad["interrupt_response"] != (cfg.WakeMode == nil || !cfg.WakeMode.Snapshot(time.Now()).Required) {
		t.Fatal(vad)
	}
	if cfg.TurnDetection == "server_vad" {
		if vad["type"] != "server_vad" || vad["silence_duration_ms"] != float64(800) || vad["eagerness"] != nil {
			t.Fatalf("従来の発話区切り設定が違います: %v", vad)
		}
	} else if vad["type"] != "semantic_vad" || vad["eagerness"] != "auto" || vad["silence_duration_ms"] != nil {
		t.Fatalf("Semantic VADの設定が違います: %v", vad)
	}
	if _, specified := session["reasoning"]; specified {
		t.Fatal("推論量はAPIのデフォルトに任せます", session)
	}
	foundWait := false
	for _, definition := range session["tools"].([]any) {
		if definition.(map[string]any)["name"] == waitToolName {
			foundWait = true
		}
	}
	if !foundWait {
		t.Fatal("待機ツールがAPIに渡されていません")
	}
	instructions, _ := session["instructions"].(string)
	if !strings.Contains(instructions, "Asia/Tokyo") || !strings.Contains(instructions, cfg.Instructions) {
		t.Fatalf("日本時間または元の指示がありません: %s", instructions)
	}
	r.send(map[string]any{"type": "session.updated"})
	r.event(types.EventRealtimeStatus)
	return r
}
func (r *rig) send(v any) {
	r.t.Helper()
	raw, _ := json.Marshal(v)
	if err := r.conn.Write(r.ctx, websocket.MessageText, raw); err != nil {
		r.t.Fatal(err)
	}
}
func (r *rig) wait(kind string) map[string]any {
	r.t.Helper()
	for {
		select {
		case m := <-r.commands:
			if m["type"] == kind {
				return m
			}
		case <-r.ctx.Done():
			r.t.Fatalf("%s が届かない", kind)
			return nil
		}
	}
}
func (r *rig) event(kind types.EventKind) types.Event {
	r.t.Helper()
	for {
		select {
		case e := <-r.events:
			if e.Kind == kind {
				return e
			}
		case <-r.ctx.Done():
			r.t.Fatalf("%v が届かない", kind)
			return types.Event{}
		}
	}
}
func (r *rig) utterance(id string) {
	r.send(map[string]any{"type": "input_audio_buffer.speech_started", "item_id": id})
	r.send(map[string]any{"type": "input_audio_buffer.speech_stopped", "item_id": id})
	r.send(map[string]any{"type": "input_audio_buffer.committed", "item_id": id})
}
func (r *rig) response(id, item, text string, metadata any) {
	r.send(map[string]any{"type": "response.created", "response": map[string]any{"id": id, "metadata": metadata}})
	r.send(map[string]any{"type": "conversation.item.added", "previous_item_id": "user1", "item": map[string]any{"id": item}})
	r.send(map[string]any{"type": "response.output_text.delta", "response_id": id, "item_id": item, "delta": text})
	r.send(map[string]any{"type": "response.output_text.done", "response_id": id, "item_id": item, "text": text})
	r.send(map[string]any{"type": "response.done", "response": map[string]any{"id": id, "status": "completed"}})
}
func TestAudioInputAndLateTranscriptPreserveResponseGeneration(t *testing.T) {
	r := newRig(t)
	r.node.Upstream <- types.Event{Payload: types.RTCPeerAudioFrame{PCM: make([]byte, 1920), SampleRate: 48000, Channels: 1}}
	audio := r.wait("input_audio_buffer.append")
	pcm, err := base64.StdEncoding.DecodeString(audio["audio"].(string))
	if err != nil || len(pcm) != 960 {
		t.Fatal(len(pcm), err)
	}
	r.utterance("user1")
	request := r.wait("response.create")
	meta := request["response"].(map[string]any)["metadata"]
	r.response("response1", "assistant1", "こんにちは。", meta)
	text := r.event(types.EventResponseText).Payload.(types.ResponseText)
	if text.GenerationID != 1 || text.Text != "こんにちは。" {
		t.Fatal(text)
	}
	r.send(map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": "user1", "transcript": "おはよう"})
	for {
		e := r.event(types.EventRealtimeOutput)
		if e.Payload.(types.OutputLine).Role == types.RoleUser {
			break
		}
	}
	records := r.records()
	if records[0].Text != "おはよう" || records[1].Role != types.RoleAgent {
		t.Fatal(records)
	}
}
func TestRepairCompletesBeforeNextResponse(t *testing.T) {
	r := newRig(t)
	r.utterance("user1")
	request := r.wait("response.create")
	r.response("response1", "assistant1", "未送信の文章。", request["response"].(map[string]any)["metadata"])
	r.event(types.EventResponseText)
	r.utterance("user2")
	deleted := r.wait("conversation.item.delete")
	if deleted["item_id"] != "assistant1" {
		t.Fatal(deleted)
	}
	replacement := r.wait("conversation.item.create")
	item := replacement["item"].(map[string]any)
	// 反映通知を受けるまでは次のresponse.createを送らない。
	select {
	case cmd := <-r.commands:
		t.Fatalf("補正確認前に送信した: %v", cmd)
	case <-time.After(30 * time.Millisecond):
	}
	r.send(map[string]any{"type": "conversation.item.deleted", "item_id": "assistant1"})
	r.send(map[string]any{"type": "conversation.item.added", "item": item})
	next := r.wait("response.create")
	if next["response"].(map[string]any)["metadata"].(map[string]any)["generation"] != "2" {
		t.Fatal(next)
	}
}
func TestOldToolResultStartsCurrentResponseAfterAcknowledgement(t *testing.T) {
	r := newRig(t)
	r.utterance("user1")
	req := r.wait("response.create")
	r.send(map[string]any{"type": "response.created", "response": map[string]any{"id": "r1", "metadata": req["response"].(map[string]any)["metadata"]}})
	r.send(map[string]any{"type": "response.function_call_arguments.done", "response_id": "r1", "call_id": "call1", "name": "test", "arguments": "{}"})
	r.event(types.EventToolRequest)
	r.send(map[string]any{"type": "response.done", "response": map[string]any{"id": "r1"}})
	r.utterance("user2")
	for {
		change := r.event(types.EventGenerationChanged).Payload.(types.GenerationChanged)
		if change.GenerationID == 2 {
			break
		}
	}
	ack := r.wait("response.create")
	r.send(map[string]any{"type": "response.created", "response": map[string]any{"id": "ack", "metadata": ack["response"].(map[string]any)["metadata"]}})
	r.send(map[string]any{"type": "response.done", "response": map[string]any{"id": "ack", "status": "completed"}})
	result := types.ToolResultRecord{ToolCallID: "call1", Name: "test", GenerationID: 1, Output: json.RawMessage(`{"ok":true}`)}
	r.node.Upstream <- types.Event{Payload: types.ConversationCommitRequest{ToolResult: &result}}
	r.wait("conversation.item.create")
	for {
		e := r.event(types.EventRealtimeOutput)
		if e.Payload.(types.OutputLine).Role == types.RoleToolResult {
			break
		}
	}
	records := r.records()
	found := false
	for _, rec := range records {
		if rec.Role == types.RoleToolResult {
			found = true
		}
	}
	if !found {
		t.Fatal(records)
	}
	next := r.wait("response.create")["response"].(map[string]any)
	if next["metadata"].(map[string]any)["generation"] != "2" || !strings.Contains(next["instructions"].(string), "call1") {
		t.Fatal("旧結果を現在の世代で説明する指示がありません", next)
	}
}

func TestToolCallArrivingAfterGenerationChangeIsExecuted(t *testing.T) {
	r := newRig(t)
	r.utterance("user1")
	req := r.wait("response.create")
	r.send(map[string]any{"type": "response.created", "response": map[string]any{"id": "r1", "metadata": req["response"].(map[string]any)["metadata"]}})
	// 引数が確定する前にユーザーが話し始め、世代が進む。
	r.utterance("user2")
	for {
		change := r.event(types.EventGenerationChanged).Payload.(types.GenerationChanged)
		if change.GenerationID == 2 {
			break
		}
	}
	r.send(map[string]any{"type": "response.function_call_arguments.done", "response_id": "r1", "call_id": "call1", "name": "set_whiteboard", "arguments": `{"content":"メモ"}`})
	tool := r.event(types.EventToolRequest).Payload.(types.ToolRequest)
	if tool.ToolCallID != "call1" || tool.GenerationID != 1 {
		t.Fatal(tool)
	}
}
func TestInitialTextIsRetainedWhileConnecting(t *testing.T) {
	r := newRigWithInitialText(t, "初回の質問")
	cmd := r.wait("conversation.item.create")
	item := cmd["item"].(map[string]any)
	if item["content"].([]any)[0].(map[string]any)["text"] != "初回の質問" {
		t.Fatal(cmd)
	}
	r.wait("response.create")
}
func TestAPIErrorLeavesPendingStateAndStopsOldPlayback(t *testing.T) {
	r := newRig(t)
	r.utterance("user1")
	r.wait("response.create")
	r.send(map[string]any{"type": "error", "error": map[string]any{"code": "invalid_request_error", "message": "生成要求が拒否された"}})
	for {
		e := r.event(types.EventRealtimeStatus)
		p := e.Payload.(types.RealtimeStatus)
		if p.Status == "error" {
			break
		}
	}
	for {
		e := r.event(types.EventGenerationChanged)
		if e.Payload.(types.GenerationChanged).GenerationID == 2 {
			break
		}
	}
	// disconnectで永久待ちを解消し、次回の接続から文脈を復元できる。
}
func TestIdleNotifiesConversationManagerOnceWithoutReset(t *testing.T) {
	ctx := context.Background()
	s := &stage{cfg: Config{IdleTimeout: time.Millisecond, RotateAfter: time.Hour}, lastActivity: time.Now().Add(-time.Second), started: time.Now(), ready: true, transport: &transport{}, node: &graph.Stage{Downstream: make(chan types.Event, 10)}}
	s.tick(ctx, time.Now())
	if e := <-s.node.Downstream; e.Kind != types.EventConversationIdle {
		t.Fatal(e)
	}
	s.tick(ctx, time.Now())
	select {
	case e := <-s.node.Downstream:
		t.Fatal("リセットまたは二重通知", e)
	default:
	}
	if s.transport == nil {
		t.Fatal("無操作で接続を張り直しました")
	}
}

func TestQueuedOldConnectionEventsAreIgnoredImmediatelyAfterDisconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	gen := generation.NewStore()
	gen.Next()
	s := &stage{cfg: Config{Generation: gen}, controller: generationcontroller.New(gen), queue: eventqueue.New[incoming](nil), epoch: 1, ready: true, peers: map[string]bool{}, responses: map[string]*response{"old": {generation: 1}}, items: map[string]*assistantItem{}, pendingOutput: map[string]bool{}}
	s.node = &graph.Stage{Upstream: make(chan types.Event), Downstream: make(chan types.Event, 10)}
	s.queue.Push(incoming{epoch: 1, err: fmt.Errorf("接続切断")})
	s.queue.Push(incoming{epoch: 1, data: []byte(`{"type":"response.output_text.delta","response_id":"old","item_id":"late","delta":"旧接続の後着"}`)})
	s.queue.Push(incoming{epoch: 1, data: []byte(`{"type":"session.updated"}`)})
	done := make(chan struct{})
	go func() { s.run(ctx); close(done) }()
	select {
	case <-s.node.Downstream:
	case <-ctx.Done():
		t.Fatal("切断通知なし")
	}
	// タイマーイベントを後続に入れ、旧データをすべて処理した後に終了する。
	s.queue.Push(incoming{event: time.Now()})
	select {
	case e := <-s.node.Downstream:
		if e.Kind != types.EventGenerationChanged {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal("停止通知なし")
	}
	cancel()
	<-done
	if _, exists := s.items["late"]; exists {
		t.Fatal("旧接続の音声イベントが再投入された")
	}
}
func TestFullySentParagraphsDoNotTriggerInterruptionRepair(t *testing.T) {
	r := newRig(t)
	r.utterance("user1")
	req := r.wait("response.create")
	r.response("r1", "a1", "こんにちは。\n次の文章です。", req["response"].(map[string]any)["metadata"])
	r.event(types.EventResponseText)
	r.node.Upstream <- types.Event{Kind: types.EventAudioSent, Payload: types.AudioFrame{GenerationID: 1, ItemID: "a1", Final: true, Text: "こんにちは。次の文章です。"}}
	r.utterance("user2")
	// delete/createを待ち飛ばさず、次のコマンドが直接response.createであることを確認する。
	select {
	case cmd := <-r.commands:
		if cmd["type"] != "response.create" {
			t.Fatal("全文送信後に不要な中断補正", cmd)
		}
	case <-r.ctx.Done():
		t.Fatal("次の応答が始まらない")
	}
}

func TestReconnectRestoresTextHistoryAndAcceptsNextTurn(t *testing.T) {
	r := newRig(t, func(cfg *Config) { cfg.Instructions = "元のシステム指示" })
	r.utterance("user1")
	r.wait("response.create")
	r.send(map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": "user1", "transcript": "前の会話を覚えて"})
	for {
		line := r.event(types.EventRealtimeOutput).Payload.(types.OutputLine)
		if line.Role == types.RoleUser {
			break
		}
	}
	r.send(map[string]any{"type": "error", "error": map[string]any{"code": "server_error", "message": "一時的な切断"}})
	select {
	case r.conn = <-r.connections:
	case <-r.ctx.Done():
		t.Fatal("再接続されない")
	}
	update := r.wait("session.update")
	if strings.Contains(update["session"].(map[string]any)["instructions"].(string), "前の会話を覚えて") {
		t.Fatal(update)
	}
	r.send(map[string]any{"type": "session.updated"})
	restored := r.wait("conversation.item.create")["item"].(map[string]any)
	if text := restored["content"].([]any)[0].(map[string]any)["text"].(string); !strings.Contains(text, "前の会話を覚えて") {
		t.Fatal(text)
	}
	r.utterance("user2")
	request := r.wait("response.create")
	if request["response"].(map[string]any)["metadata"].(map[string]any)["generation"] != "3" {
		t.Fatal(request)
	}
	instructions, _ := request["response"].(map[string]any)["instructions"].(string)
	for _, want := range []string{"元のシステム指示", "Asia/Tokyo", "今日:"} {
		if !strings.Contains(instructions, want) {
			t.Fatalf("応答ごとの指示に %q がありません: %s", want, instructions)
		}
	}
}
func TestRotationWaitsForAudioSendCompletion(t *testing.T) {
	r := newRig(t, func(cfg *Config) { cfg.RotateAfter = time.Millisecond })
	r.utterance("user1")
	req := r.wait("response.create")
	r.response("r1", "a1", "まだ音声送信中。", req["response"].(map[string]any)["metadata"])
	r.event(types.EventResponseText)
	select {
	case <-r.connections:
		t.Fatal("音声送信中に接続更新された")
	case <-time.After(1100 * time.Millisecond):
	}
	r.node.Upstream <- types.Event{Kind: types.EventAudioSent, Payload: types.AudioFrame{ItemID: "a1", GenerationID: 1, End: true}}
	select {
	case r.conn = <-r.connections:
	case <-r.ctx.Done():
		t.Fatal("送信終了後に接続更新されない")
	}
	r.wait("session.update")
	r.send(map[string]any{"type": "session.updated"})
}

func TestReadResultPromptsOptionalWhiteboardAndThenReturnsToSpeech(t *testing.T) {
	r := newRig(t, func(cfg *Config) {
		cfg.Tools = []any{map[string]any{"type": "function", "name": "set_whiteboard", "parameters": map[string]any{"type": "object", "properties": map[string]any{"content": map[string]any{"type": "string"}}}}}
	})
	r.utterance("user1")
	initial := r.wait("response.create")
	created := func(id string, req map[string]any) {
		r.send(map[string]any{"type": "response.created", "response": map[string]any{"id": id, "metadata": req["response"].(map[string]any)["metadata"]}})
	}
	call := func(responseID, callID, name string) {
		r.send(map[string]any{"type": "response.function_call_arguments.done", "response_id": responseID, "call_id": callID, "name": name, "arguments": "{}"})
		r.event(types.EventToolRequest)
		r.send(map[string]any{"type": "response.done", "response": map[string]any{"id": responseID}})
	}
	result := func(callID, name, output string) {
		p := types.ToolResultRecord{ToolCallID: callID, Name: name, GenerationID: 1, Output: json.RawMessage(output)}
		r.node.Upstream <- types.Event{Payload: types.ConversationCommitRequest{ToolResult: &p}}
		r.wait("conversation.item.create")
	}
	created("r1", initial)
	call("r1", "search1", "web_search")
	result("search1", "web_search", `{"result":"明日の天気"}`)
	required := r.wait("response.create")
	followup := required["response"].(map[string]any)
	if followup["tool_choice"] != nil || !strings.Contains(followup["instructions"].(string), "口頭説明より先に") {
		t.Fatal("表示の必要性を判断する指示がありません", required)
	}
	created("r2", required)
	call("r2", "board1", "set_whiteboard")
	result("board1", "set_whiteboard", `{"updated":true}`)
	speech := r.wait("response.create")
	if _, forced := speech["response"].(map[string]any)["tool_choice"]; forced {
		t.Fatal("表示後もホワイトボードを繰り返す", speech)
	}
	created("r3", speech)
	r.send(map[string]any{"type": "response.done", "response": map[string]any{"id": "r3"}})
	r.utterance("user2")
	manual := r.wait("response.create")
	if manual["response"].(map[string]any)["tool_choice"] != nil {
		t.Fatal("追加の表示依頼で不要なツールが強制されました", manual)
	}
	created("r4", manual)
	r.send(map[string]any{"type": "response.function_call_arguments.done", "response_id": "r4", "call_id": "board-detail", "name": "set_whiteboard", "arguments": `{"content":"取得済みの予報を詳しく表示"}`})
	detail := r.event(types.EventToolRequest).Payload.(types.ToolRequest)
	if detail.Name != "set_whiteboard" || detail.GenerationID != 2 || string(detail.Arguments) != `{"content":"取得済みの予報を詳しく表示"}` {
		t.Fatalf("追加表示の呼び出しが受理されませんでした: %#v", detail)
	}
}

func TestStaleHistorySyncReleasesWriter(t *testing.T) {
	s := newToolResponseStage(t)
	done := make(chan struct{})
	s.handle(context.Background(), types.Event{Kind: types.EventConversationHistorySync, Payload: types.ConversationHistorySync{Epoch: s.epoch - 1, Done: done}})
	select {
	case <-done:
	default:
		t.Fatal("旧接続の同期で会話管理が止まる")
	}
	noToolCommand(t, s)
}

func TestConnectReadsLongTermMemoryDirectly(t *testing.T) {
	memory, err := memorystate.NewStore(filepath.Join(t.TempDir(), "memories.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = memory.Upsert(memorystate.UpsertInput{Content: "好きな季節は秋"}); err != nil {
		t.Fatal(err)
	}
	r := newRig(t, func(cfg *Config) { cfg.Memory = memory })
	item := r.wait("conversation.item.create")["item"].(map[string]any)
	text := item["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "好きな季節は秋") {
		t.Fatal(text)
	}
}

// 記録の観測時には保存・配送を待つ。配送を担当するgoroutine自体は待機させない。
func (r *rig) records() []types.ConversationRecord {
	done := make(chan struct{})
	r.managerQueue.Push(types.Event{Kind: types.EventConversationHistorySync, Payload: types.ConversationHistorySync{Done: done}})
	select {
	case <-done:
	case <-r.ctx.Done():
		r.t.Fatal("履歴の保存を待てない")
	}
	return r.history.Snapshot()
}
