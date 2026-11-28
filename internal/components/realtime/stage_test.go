package realtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/tetetratra/smart-speaker/internal/components/conversationrecorder"
	"github.com/tetetratra/smart-speaker/internal/components/generationcontroller"
	"github.com/tetetratra/smart-speaker/internal/eventqueue"
	"github.com/tetetratra/smart-speaker/internal/features/memory"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/types"
	"net/http"
	"net/http/httptest"
	"nhooyr.io/websocket"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// rig は、偽の Realtime API サーバーと記録係（conversationrecorder）をつないで、文字起こしと応答生成の2つのコンポーネントを動かす。
// 会話の流れ（conversationflow）の代わりに、応答を求めない記録だけを応答生成側へ渡し、utterance では応答依頼を送る。
type rig struct {
	managerQueue *eventqueue.Queue[types.Event]
	gen          *generation.Store
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
	return createRig(t, "", nil, options...)
}
func newRigWithInbound(t *testing.T, inbound <-chan types.ConversationInput, options ...func(*Config)) *rig {
	return createRig(t, "", inbound, options...)
}
func newTestHistory(t *testing.T) *conversationhistory.Store {
	t.Helper()
	history, err := conversationhistory.NewStore(filepath.Join(t.TempDir(), "conversation_history.json"))
	if err != nil {
		t.Fatal(err)
	}
	return history
}

func createRig(t *testing.T, initialText string, inbound <-chan types.ConversationInput, options ...func(*Config)) *rig {
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
	r.gen = cfg.Generation
	transcription, response, err := NewStages(cfg)
	if err != nil {
		t.Fatal(err)
	}
	// テストからは1つの入口に送り、イベントの種類で文字起こし側か応答生成側へ振り分ける。
	node := &graph.Stage{Upstream: make(chan types.Event)}
	r.node = node
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case e := <-node.Upstream:
				to := transcription
				switch e.(type) {
				case types.EventResponseRequest, types.EventConversationInput, types.EventConversationHistorySync, types.EventAudioSent:
					to = response
				}
				select {
				case to.Upstream <- e:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	manager := conversationrecorder.NewStage(conversationrecorder.Config{History: r.history})
	managerQueue := eventqueue.New[types.Event](nil)
	r.managerQueue = managerQueue
	go manager.Run(ctx)
	// 外からの入力は、gateway と同じく会話管理へイベントとして届ける。
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case input, ok := <-inbound:
				if !ok {
					return
				}
				managerQueue.Push(types.EventConversationInput{ConversationInput: input})
			}
		}
	}()
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
				// 会話の流れと同じく、応答を求めない記録と履歴の同期だけを応答生成側へ渡す。
				if input, ok := e.(types.EventConversationInput); ok && (input.ModelAlreadyKnows || input.ModelShouldRespond) {
					continue
				}
				select {
				case response.Upstream <- e:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	go transcription.Run(ctx)
	go response.Run(ctx)
	for _, out := range []*graph.Stage{transcription, response} {
		go func(out *graph.Stage) {
			for {
				select {
				case <-ctx.Done():
					return
				case e, ok := <-out.Downstream:
					if !ok {
						return
					}
					switch e.(type) {
					case types.EventConversationInput, types.EventConversationHistorySync, types.EventConversationIdle, types.EventConversationRecordsUsed:
						managerQueue.Push(e)
					}

					select {
					case r.events <- e:
					case <-ctx.Done():
						return
					}
				}
			}
		}(out)
	}
	if initialText == "__offline__" {
		return r
	}
	node.Upstream <- types.EventRTCPeerOutputSink{RTCPeerOutputSink: types.RTCPeerOutputSink{PeerID: "test", Connected: true}}
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
	wantNoiseReduction := "far_field"
	if cfg.NoiseReduction != "" {
		wantNoiseReduction = cfg.NoiseReduction
	}
	if nr := session["audio"].(map[string]any)["input"].(map[string]any)["noise_reduction"].(map[string]any); nr["type"] != wantNoiseReduction {
		t.Fatalf("ノイズ除去の設定が違います: %v", nr)
	}
	if _, specified := session["reasoning"]; specified {
		t.Fatal("推論量はAPIのデフォルトに任せます", session)
	}
	if _, ok := session["tools"]; ok {
		t.Fatal("Realtime にツールを渡しています。ツールは頭脳AIだけが持ちます", session["tools"])
	}
	instructions, _ := session["instructions"].(string)
	if strings.Contains(instructions, "Asia/Tokyo") || !strings.Contains(instructions, cfg.Instructions) {
		t.Fatalf("指示に日時が入っているか、元の指示がありません: %s", instructions)
	}
	r.send(map[string]any{"type": "session.updated"})
	r.event(types.EventRealtimeStatus{})
	return r
}
func (r *rig) send(v any) {
	r.t.Helper()
	raw, _ := json.Marshal(v)
	if err := r.conn.Write(r.ctx, websocket.MessageText, raw); err != nil {
		r.t.Fatal(err)
	}
}

// waitResponse は次の応答要求まで待ち、その直前に足した知らせの本文も返す。
func (r *rig) waitResponse() (map[string]any, string) {
	r.t.Helper()
	notice := ""
	for {
		select {
		case m := <-r.commands:
			if isNotice(m) {
				notice = noticeText(m)
			}
			if m["type"] == "response.create" {
				return m["response"].(map[string]any), notice
			}
		case <-r.ctx.Done():
			r.t.Fatal("response.create が届かない")
			return nil, ""
		}
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

// event は want と同じ型のイベントが届くまで待つ。want にはゼロ値を渡す。
func (r *rig) event(want types.Event) types.Event {
	r.t.Helper()
	for {
		select {
		case e := <-r.events:
			if reflect.TypeOf(e) == reflect.TypeOf(want) {
				return e
			}
		case <-r.ctx.Done():
			r.t.Fatalf("%s が届かない", types.EventName(want))
			return nil
		}
	}
}

// utterance はユーザーが1回話したことにし、会話の流れが「Realtime がそのまま返す」と判定したときと同じく、応答依頼を送る。
func (r *rig) utterance(id string) {
	before := r.gen.Current()
	r.send(map[string]any{"type": "input_audio_buffer.speech_started", "item_id": id})
	r.send(map[string]any{"type": "input_audio_buffer.speech_stopped", "item_id": id})
	r.send(map[string]any{"type": "input_audio_buffer.committed", "item_id": id})
	r.respond(before, "")
}

// respond は、世代が before から進むのを待ってから、今の世代への応答依頼を送る。
func (r *rig) respond(before types.GenerationID, notice string) {
	r.t.Helper()
	deadline := time.Now().Add(time.Second)
	for r.gen.Current() == before && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	r.node.Upstream <- types.EventResponseRequest{ResponseRequest: types.ResponseRequest{GenerationID: r.gen.Current(), Notice: notice}}
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
	r.node.Upstream <- types.EventRTCPeerAudioFrame{RTCPeerAudioFrame: types.RTCPeerAudioFrame{PCM: make([]byte, 1920), SampleRate: 48000, Channels: 1}}
	audio := r.wait("input_audio_buffer.append")
	pcm, err := base64.StdEncoding.DecodeString(audio["audio"].(string))
	if err != nil || len(pcm) != 960 {
		t.Fatal(len(pcm), err)
	}
	r.utterance("user1")
	request := r.wait("response.create")
	meta := request["response"].(map[string]any)["metadata"]
	r.response("response1", "assistant1", "こんにちは。", meta)
	text := r.event(types.EventResponseText{}).(types.EventResponseText).ResponseText
	if text.GenerationID != 1 || text.Text != "こんにちは。" {
		t.Fatal(text)
	}
	r.send(map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": "user1", "transcript": "おはよう"})
	for {
		input := r.event(types.EventConversationInput{}).(types.EventConversationInput)
		if input.Record.Role == types.RoleUser {
			break
		}
	}
	// 応答は読み上げの区切りを送り終えたときに記録するので、文字起こしの後ろに並ぶ。
	r.node.Upstream <- types.EventAudioSent{AudioFrame: types.AudioFrame{GenerationID: 1, ItemID: "assistant1", SegmentID: "s1", Final: true, Text: "こんにちは。"}}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		records := r.records()
		if len(records) == 2 {
			if records[0].Role != types.RoleUser || records[0].Text != "おはよう" || records[1].Role != types.RoleAgent || records[1].Text != "こんにちは。" {
				t.Fatal(records)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("記録が揃いません", r.records())
}
func TestAPIErrorLeavesPendingStateAndStopsOldPlayback(t *testing.T) {
	r := newRig(t)
	r.utterance("user1")
	r.wait("response.create")
	r.send(map[string]any{"type": "error", "error": map[string]any{"code": "invalid_request_error", "message": "生成要求が拒否された"}})
	for {
		e := r.event(types.EventRealtimeStatus{})
		p := e.(types.EventRealtimeStatus).RealtimeStatus
		if p.Status == "error" {
			break
		}
	}
	for {
		e := r.event(types.EventGenerationChanged{})
		if e.(types.EventGenerationChanged).GenerationChanged.GenerationID == 2 {
			break
		}
	}
	// disconnectで永久待ちを解消し、次回の接続から文脈を復元できる。
}
func TestIdleNotifiesConversationManagerOnceWithoutReset(t *testing.T) {
	ctx := context.Background()
	s := &stage{cfg: Config{IdleTimeout: time.Millisecond, RotateAfter: time.Hour}, lastActivity: time.Now().Add(-time.Second), started: time.Now(), ready: true, transport: &transport{}, response: &graph.Stage{Downstream: make(chan types.Event, 10)}}
	s.tick(ctx, time.Now())
	if e := <-s.response.Downstream; !types.Is[types.EventConversationIdle](e) {
		t.Fatal(e)
	}
	s.tick(ctx, time.Now())
	select {
	case e := <-s.response.Downstream:
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
	s.transcription = &graph.Stage{Upstream: make(chan types.Event), Downstream: make(chan types.Event, 10)}
	s.response = &graph.Stage{Upstream: make(chan types.Event), Downstream: make(chan types.Event, 10)}
	s.queue.Push(incoming{epoch: 1, err: fmt.Errorf("接続切断")})
	s.queue.Push(incoming{epoch: 1, data: []byte(`{"type":"response.output_text.delta","response_id":"old","item_id":"late","delta":"旧接続の後着"}`)})
	s.queue.Push(incoming{epoch: 1, data: []byte(`{"type":"session.updated"}`)})
	done := make(chan struct{})
	go func() { s.run(ctx); close(done) }()
	select {
	case <-s.transcription.Downstream:
	case <-ctx.Done():
		t.Fatal("切断通知なし")
	}
	// タイマーイベントを後続に入れ、旧データをすべて処理した後に終了する。
	s.queue.Push(incoming{event: time.Now()})
	select {
	case e := <-s.transcription.Downstream:
		if !types.Is[types.EventGenerationChanged](e) {
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
	r.event(types.EventResponseText{})
	r.node.Upstream <- types.EventAudioSent{AudioFrame: types.AudioFrame{GenerationID: 1, ItemID: "a1", Final: true, Text: "こんにちは。次の文章です。"}}
	r.utterance("user2")
	// delete/createを待ち飛ばさず、日時の知らせの次のコマンドが直接response.createであることを確認する。
	for {
		select {
		case cmd := <-r.commands:
			if isNotice(cmd) {
				continue
			}
			if cmd["type"] != "response.create" {
				t.Fatal("全文送信後に不要な中断補正", cmd)
			}
			return
		case <-r.ctx.Done():
			t.Fatal("次の応答が始まらない")
		}
	}
}

func TestReconnectRestoresTextHistoryAndAcceptsNextTurn(t *testing.T) {
	r := newRig(t, func(cfg *Config) { cfg.Instructions = "元のシステム指示" })
	r.utterance("user1")
	r.wait("response.create")
	r.send(map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": "user1", "transcript": "前の会話を覚えて"})
	for {
		input := r.event(types.EventConversationInput{}).(types.EventConversationInput)
		if input.Record.Role == types.RoleUser {
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
	if _, ok := request["response"].(map[string]any)["instructions"]; ok {
		t.Fatalf("応答ごとに指示を変えています: %v", request)
	}
}
func TestRotationWaitsForAudioSendCompletion(t *testing.T) {
	r := newRig(t, func(cfg *Config) { cfg.RotateAfter = time.Millisecond })
	r.utterance("user1")
	req := r.wait("response.create")
	r.response("r1", "a1", "まだ音声送信中。", req["response"].(map[string]any)["metadata"])
	r.event(types.EventResponseText{})
	select {
	case <-r.connections:
		t.Fatal("音声送信中に接続更新された")
	case <-time.After(1100 * time.Millisecond):
	}
	r.node.Upstream <- types.EventAudioSent{AudioFrame: types.AudioFrame{ItemID: "a1", GenerationID: 1, End: true}}
	select {
	case r.conn = <-r.connections:
	case <-r.ctx.Done():
		t.Fatal("送信終了後に接続更新されない")
	}
	r.wait("session.update")
	r.send(map[string]any{"type": "session.updated"})
}

func TestStaleHistorySyncReleasesWriter(t *testing.T) {
	s := newTestStage(t)
	done := make(chan struct{})
	s.handle(context.Background(), types.EventConversationHistorySync{Epoch: s.epoch - 1, Done: done})
	select {
	case <-done:
	default:
		t.Fatal("旧接続の同期で記録係が止まる")
	}
	noCommand(t, s)
}

func TestConnectReadsLongTermMemoryDirectly(t *testing.T) {
	memoryStore, err := memory.NewStore(filepath.Join(t.TempDir(), "memories.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = memoryStore.Upsert(memory.UpsertInput{Content: "好きな季節は秋"}); err != nil {
		t.Fatal(err)
	}
	r := newRig(t, func(cfg *Config) { cfg.Memory = memoryStore })
	item := r.wait("conversation.item.create")["item"].(map[string]any)
	text := item["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "好きな季節は秋") {
		t.Fatal(text)
	}
}

// 記録の観測時には保存・配送を待つ。配送を担当するgoroutine自体は待機させない。
// waitRecord は、ID の記録が保存されるまで待つ。
func (r *rig) waitRecord(id string) {
	r.t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		for _, record := range r.records() {
			if record.ID == id {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.t.Fatal("記録が保存されません", id, r.records())
}

func (r *rig) records() []types.ConversationRecord {
	done := make(chan struct{})
	r.managerQueue.Push(types.EventConversationHistorySync{Done: done})
	select {
	case <-done:
	case <-r.ctx.Done():
		r.t.Fatal("履歴の保存を待てない")
	}
	return r.history.Snapshot()
}

// ユーザーの発話は、記録係から画面に送られる（チャット欄に出す）よう UserAlreadyKnows を立てない。
// 文字起こしが空の発話は見せる中身がないので立てる。
func TestUserUtteranceRecordsAreShownOnScreen(t *testing.T) {
	userInput := func(r *rig) types.ConversationInput {
		for {
			input := r.event(types.EventConversationInput{}).(types.EventConversationInput).ConversationInput
			if input.Record.Role == types.RoleUser {
				return input
			}
		}
	}
	r := newRig(t)
	r.utterance("user1")
	r.wait("response.create")
	r.send(map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": "user1", "transcript": "おはよう"})
	if input := userInput(r); input.Record.Text != "おはよう" || input.Record.GenerationID == 0 || input.UserAlreadyKnows {
		t.Fatal("発話が画面に出ません", input)
	}
	r = newRig(t)
	r.utterance("user2")
	r.wait("response.create")
	r.send(map[string]any{"type": "conversation.item.input_audio_transcription.completed", "item_id": "user2", "transcript": ""})
	if input := userInput(r); input.Record.Text != "" || !input.UserAlreadyKnows {
		t.Fatal("空の発話が画面に出ます", input)
	}
}
