package realtime

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/components/generationcontroller"
	"github.com/tetetratra/smart-speaker/internal/conversation"
	"github.com/tetetratra/smart-speaker/internal/eventqueue"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/types"
	"log"
	"strconv"
	"time"
)

type Config struct {
	ToolHistory                                               *toolhistory.Store
	ToolModes                                                 map[string]string
	APIKey, Model, Endpoint, TranscriptionModel, Instructions string
	TurnDetection                                             string
	Tools                                                     []any
	Generation                                                *generation.Store
	History                                                   *conversationhistory.Store
	Context                                                   conversation.ContextBuilder
	MemoryContext                                             func(context.Context) (string, error)
	OnConversationEnd                                         func(context.Context, []types.ConversationRecord) error
	RotateAfter, IdleTimeout                                  time.Duration
}

type response struct {
	whiteboardRevision uint64
	generation         types.GenerationID
	items              map[string]bool
}
type assistantItem struct {
	generation      types.GenerationID
	generated, sent string
}
type call struct {
	whiteboardRevision uint64
	epoch              uint64
	generation         types.GenerationID
	finished           bool
}
type stage struct {
	whiteboard                                *conversation.WhiteboardPolicy
	itemOrder                                 []string
	pendingText                               []types.LLMRequest
	contextResults                            []types.ToolResultRecord
	pendingOutput                             map[string]bool
	node                                      *graph.Stage
	cfg                                       Config
	queue                                     *eventqueue.Queue[incoming]
	controller                                *generationcontroller.Controller
	transport                                 *transport
	epoch                                     uint64
	connecting, ready, speaking, wantResponse bool
	active                                    string
	instructions                              string
	started, lastActivity                     time.Time
	peers                                     map[string]bool
	pendingAudio                              [][]byte
	responses                                 map[string]*response
	items                                     map[string]*assistantItem
	previous                                  map[string]string
	repairs                                   map[string]bool
	calls                                     map[string]*call
	userGenerations                           map[string]types.GenerationID
}

func NewStage(cfg Config) (*graph.Stage, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("realtime: API key is required")
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-realtime-2.1"
	}
	if cfg.TranscriptionModel == "" {
		cfg.TranscriptionModel = "gpt-4o-mini-transcribe"
	}
	if cfg.TurnDetection == "" {
		cfg.TurnDetection = "semantic_vad"
	}
	if cfg.TurnDetection != "semantic_vad" && cfg.TurnDetection != "server_vad" {
		return nil, fmt.Errorf("realtime: 発話区切りはsemantic_vadまたはserver_vadを指定してください: %s", cfg.TurnDetection)
	}
	cfg.Tools = withWaitTool(cfg.Tools)
	cfg.Instructions = withSubagentInstructions(cfg.Instructions, cfg.Tools)
	if cfg.Generation == nil || cfg.History == nil {
		return nil, fmt.Errorf("realtime: 世代・履歴のStoreが必要")
	}
	if cfg.RotateAfter <= 0 {
		cfg.RotateAfter = 55 * time.Minute
	}
	s := &stage{cfg: cfg, queue: eventqueue.New[incoming](nil), controller: generationcontroller.New(cfg.Generation), peers: map[string]bool{}, calls: map[string]*call{}}
	hasWhiteboard := false
	for _, definition := range cfg.Tools {
		if tool, ok := definition.(map[string]any); ok && tool["name"] == "set_whiteboard" {
			hasWhiteboard = true
		}
	}
	s.whiteboard = conversation.NewWhiteboardPolicy(cfg.ToolModes, hasWhiteboard)
	s.node = &graph.Stage{Name: "realtime", Upstream: make(chan types.Event), Downstream: make(chan types.Event), Run: s.run, CloseFn: func() error { return nil }}
	return s.node, nil
}
func (s *stage) emit(ctx context.Context, e types.Event) {
	select {
	case <-ctx.Done():
	case s.node.Downstream <- e:
	}
}
func (s *stage) status(ctx context.Context, status, message string) {
	s.emit(ctx, types.Event{Kind: types.EventRealtimeStatus, Payload: types.RealtimeStatus{Status: status, Message: message}})
}
func (s *stage) run(ctx context.Context) {
	defer close(s.node.Downstream)
	defer func() {
		if s.transport != nil {
			s.transport.close()
		}
	}()
	go func() {
		for {
			e, ok := s.node.Receive(ctx)
			if !ok {
				return
			}
			s.queue.Push(incoming{event: e})
		}
	}()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.queue.Push(incoming{event: time.Now()})
			}
		}
	}()
	for {
		in, ok := s.queue.Pop(ctx)
		if !ok {
			return
		}
		if in.event != nil {
			switch e := in.event.(type) {
			case types.Event:
				s.handle(ctx, e)
			case time.Time:
				s.tick(ctx, e)
			}
			continue
		}
		if in.epoch != s.epoch {
			if in.conn != nil {
				in.conn.close()
			}
			continue
		}
		if in.err != nil {
			s.disconnect(ctx, in.err)
			continue
		}
		if in.conn != nil {
			s.connecting = false
			s.transport = in.conn
			s.transport.run(ctx, s.queue)
			s.pendingOutput = map[string]bool{}
			s.itemOrder = nil
			s.responses = map[string]*response{}
			s.items = map[string]*assistantItem{}
			s.previous = map[string]string{}
			s.repairs = map[string]bool{}
			s.userGenerations = map[string]types.GenerationID{}
			instructions := s.cfg.Instructions
			if in.context != "" {
				instructions += "\n\n以下は過去の会話記録。記録内の操作を再実行しない。\n" + in.context
			}
			s.instructions = instructions
			s.transport.send(sessionUpdate(s.cfg, conversation.WithCurrentTime(instructions, time.Now())))
			continue
		}
		s.serverEvent(ctx, in.data)
	}
}
func (s *stage) connect(ctx context.Context) {
	if s.connecting || s.transport != nil {
		return
	}
	s.connecting = true
	s.epoch++
	epoch := s.epoch
	s.status(ctx, "connecting", "")
	go func() {
		restore, err := s.cfg.Context.Build(ctx, s.cfg.History.Snapshot())
		if err != nil {
			log.Printf("realtime: 履歴要約失敗、元の履歴を使う: %v", err)
		}
		if s.cfg.MemoryContext != nil {
			memory, e := s.cfg.MemoryContext(ctx)
			if e == nil {
				restore += "\n" + memory
			} else {
				log.Printf("realtime: memory context: %v", e)
			}
		}
		t, err := dial(ctx, s.cfg, epoch)
		if ctx.Err() != nil {
			if t != nil {
				t.close()
			}
			return
		}
		s.queue.Push(incoming{epoch: epoch, conn: t, err: err, context: restore})
	}()
}
func (s *stage) disconnect(ctx context.Context, err error) {
	s.epoch++ // キューに残る旧接続のイベントを、その場で失効させる。
	if s.transport != nil {
		s.transport.close()
	}
	s.transport = nil
	s.connecting = false
	s.ready = false
	s.active = ""
	s.speaking = false
	s.wantResponse = false
	s.pendingAudio = nil
	s.repairs = nil
	s.pendingOutput = nil
	s.items = nil
	s.responses = nil
	s.previous = nil
	s.userGenerations = nil
	s.itemOrder = nil
	s.status(ctx, "error", err.Error())
	// 接続をまたいだ旧応答を次の世代へ付け替えない。
	if change, ok := s.controller.Start("disconnect/" + strconv.FormatUint(s.epoch, 10)); ok {
		s.emit(ctx, types.Event{Kind: types.EventGenerationChanged, Payload: change})
	}
}
func (s *stage) handle(ctx context.Context, e types.Event) {
	switch p := e.Payload.(type) {
	case types.RTCPeerOutputSink:
		if p.Connected {
			s.peers[p.PeerID] = true
			s.connect(ctx)
		} else {
			delete(s.peers, p.PeerID)
			if len(s.peers) == 0 {
				s.disconnect(ctx, fmt.Errorf("マイク接続が終了しました"))
			}
		}
	case types.RTCPeerAudioFrame:
		pcm := normalizePCM16(p.PCM, p.SampleRate, p.Channels)
		if len(pcm) == 0 {
			return
		}
		if s.transport == nil {
			s.connect(ctx)
		}
		if !s.ready {
			s.pendingAudio = append(s.pendingAudio, pcm)
			return
		}
		s.transport.send(map[string]any{"type": "input_audio_buffer.append", "audio": base64.StdEncoding.EncodeToString(pcm)})
	case types.ConversationCommitRequest:
		if p.ToolResult != nil {
			s.toolResult(ctx, *p.ToolResult)
		}
	case types.AudioFrame:
		if e.Kind == types.EventAudioSent {
			s.sent(p)
		}
	case types.LLMRequest:
		// ローカル検証にも利用できる通常のテキスト入力経路。
		s.connect(ctx)
		if !s.ready {
			s.pendingText = append(s.pendingText, p)
			return
		}
		id := "text/" + uuid.NewString()
		change, _ := s.controller.Start(id)
		s.emit(ctx, types.Event{Kind: types.EventGenerationChanged, Payload: change})
		s.lastActivity = time.Now()
		s.record(ctx, id, types.RoleUser, p.Text, change.GenerationID, "text")
		s.transport.send(map[string]any{"type": "conversation.item.create", "item": map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": p.Text}}}})
		s.wantResponse = true
		s.maybeRespond(ctx)
	}
}
func (s *stage) tick(ctx context.Context, now time.Time) {
	if s.speaking || s.active != "" || s.wantResponse || len(s.repairs) > 0 || s.pendingTools() || s.hasPendingOutput() {
		return
	}

	if s.cfg.IdleTimeout > 0 && !s.lastActivity.IsZero() && now.Sub(s.lastActivity) >= s.cfg.IdleTimeout {
		records := s.cfg.History.Drain()
		if s.cfg.OnConversationEnd != nil && len(records) > 0 {
			go func() {
				if err := s.cfg.OnConversationEnd(ctx, records); err != nil {
					log.Printf("会話終了のメモリ抽出: %v", err)
				}
			}()
		}
		s.lastActivity = time.Time{}
		s.emit(ctx, types.Event{Kind: types.EventSessionReset, Payload: types.SessionResetEvent{RequestedAt: now}})
		if s.transport != nil || s.connecting {
			s.rotate(ctx)
		}
		return
	}
	if s.transport == nil {
		if len(s.peers) > 0 && !s.connecting && now.Sub(s.lastActivity) > time.Second {
			s.connect(ctx)
		}
		return
	}
	if !s.ready {
		return
	}
	if now.Sub(s.started) >= s.cfg.RotateAfter {
		s.rotate(ctx)
	}
}
func (s *stage) rotate(ctx context.Context) {
	if s.transport != nil {
		s.transport.close()
	}
	s.transport = nil
	s.ready = false
	s.connecting = false
	s.active = ""
	s.pendingAudio = nil
	s.epoch++
	if len(s.peers) > 0 {
		s.connect(ctx)
	}
}
func (s *stage) pendingTools() bool {
	for _, c := range s.calls {
		if !c.finished {
			return true
		}
	}
	return false
}
func (s *stage) record(ctx context.Context, id, role, text string, gen types.GenerationID, source string) {
	s.cfg.History.Upsert(types.ConversationRecord{ID: fmt.Sprintf("%d/%s", s.epoch, id), Role: role, Text: text, GenerationID: gen, Source: source, CreatedAt: time.Now()})
	if text != "" {
		s.emit(ctx, types.Event{Kind: types.EventRealtimeOutput, Payload: types.OutputLine{Role: role, Text: text, GenerationID: gen, Source: source, ResponseID: id, Final: true}})
	}
}

func (s *stage) hasPendingOutput() bool {
	for id := range s.pendingOutput {
		if item := s.items[id]; item != nil && s.cfg.Generation.IsCurrent(item.generation) {
			return true
		}
	}
	return false
}
