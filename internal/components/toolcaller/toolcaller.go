package toolcaller

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/tools"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

type toolCaller struct {
	upstream        chan types.Event
	downstream      chan types.Event
	ctx             context.Context
	cancel          context.CancelFunc
	once            sync.Once
	closerWaitGroup sync.WaitGroup
	taskGroup       sync.WaitGroup

	tools   map[string]tools.Handler
	node    *graph.Stage
	history *toolhistory.Store

	// results は call_id ごとの結果。値が nil なら実行中。
	mu      sync.Mutex
	results map[string]*types.ToolResultRecord
}

// NewStage は tool の実行 stage を作る。history を渡すと、実行の開始と終了をツール呼び出し履歴に記録する。
// 呼び出しは届いた順を待たずに並行して実行する。
func NewStage(handlers map[string]tools.Handler, history *toolhistory.Store) *graph.Stage {
	if handlers == nil {
		handlers = map[string]tools.Handler{}
	}
	s := &toolCaller{
		upstream:   make(chan types.Event),
		results:    map[string]*types.ToolResultRecord{},
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		tools:      handlers,
		history:    history,
	}
	s.node = &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.close,
	}
	return s.node
}

func (s *toolCaller) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.ctx = ctx
	s.cancel = cancel
	for _, tool := range s.tools {
		if ctxTool, ok := tool.(tools.ContextAware); ok {
			ctxTool.SetContext(ctx)
		}
	}
	s.closerWaitGroup.Add(1)
	go func() {
		defer s.closerWaitGroup.Done()
		defer func() {
			s.taskGroup.Wait()
			close(s.downstream)
		}()
		for {
			evt, ok := s.node.Receive(ctx)
			if !ok {
				return
			}
			if evt.Kind != types.EventToolRequest {
				continue
			}
			req, ok := evt.Payload.(types.ToolRequest)
			if !ok {
				continue
			}
			s.taskGroup.Add(1)
			go func() {
				defer s.taskGroup.Done()
				s.dispatchTool(req)
			}()
		}
	}()
}

// dispatchTool は同じ call_id を一度だけ実行する。実行中の再送は無視し、実行済みなら前の結果を送り直す。
func (s *toolCaller) dispatchTool(req types.ToolRequest) {
	s.mu.Lock()
	result, exists := s.results[req.ToolCallID]
	if !exists {
		s.results[req.ToolCallID] = nil
	}
	s.mu.Unlock()
	if exists && result == nil {
		return
	}
	if !exists {
		var entryID string
		if s.history != nil {
			entryID = s.history.Begin(req)
		}
		log.Printf("toolcaller: 開始 name=%s call_id=%s", req.Name, req.ToolCallID)
		started := time.Now()
		record := s.executeTool(req)
		log.Printf("toolcaller: 終了 name=%s call_id=%s elapsed=%s", req.Name, req.ToolCallID, time.Since(started).Round(time.Millisecond))
		if s.history != nil {
			s.history.Finish(entryID, record)
		}
		result = &record
		s.mu.Lock()
		s.results[req.ToolCallID] = result
		s.mu.Unlock()
	}
	sent := *result // 送り直しのたびに別の値を渡し、受け手どうしで共有しない。
	s.emit(types.Event{Kind: types.EventConversationCommitRequest, Payload: types.ConversationCommitRequest{Role: types.RoleToolResult, GenerationID: sent.GenerationID, Source: sent.Name, ToolResult: &sent}})
}

func (s *toolCaller) executeTool(req types.ToolRequest) types.ToolResultRecord {
	args := map[string]any{}
	if len(req.Arguments) > 0 {
		if err := json.Unmarshal(req.Arguments, &args); err != nil {
			return types.ToolResultRecord{ToolCallID: req.ToolCallID, Name: req.Name, GenerationID: req.GenerationID, Output: json.RawMessage(`{"error":"invalid tool arguments"}`)}
		}
	}
	tool, ok := s.tools[req.Name]
	var result map[string]any
	if !ok {
		result = map[string]any{
			"error": "unknown function: " + req.Name,
		}
	} else {
		out, err := tool.Run(args)
		if err != nil {
			result = map[string]any{"error": err.Error()}
		} else {
			result = out
		}
	}
	output, err := json.Marshal(result)
	if err != nil {
		log.Printf("toolcaller: result marshal error: %v", err)
		output = []byte(`{"error":"result encoding failed"}`)
	}
	return types.ToolResultRecord{
		ToolCallID:   req.ToolCallID,
		Name:         req.Name,
		Output:       output,
		GenerationID: req.GenerationID,
	}
}

func (s *toolCaller) close() error {
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.closerWaitGroup.Wait()
		close(s.upstream)
		log.Println("toolcaller: stage closed")
	})
	return nil
}

// emit は ctx を尊重しつつ下流へイベントを送る。
func (s *toolCaller) emit(evt types.Event) {
	select {
	case <-s.ctx.Done():
		return
	case s.downstream <- evt:
	}
}
