package toolcaller

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/tools/execution"
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

	executor *execution.Executor
	node     *graph.Stage

	// results は call_id ごとの結果。値が nil なら実行中。
	mu      sync.Mutex
	results map[string]*types.ToolResultRecord
}

// NewStage は tool の実行 stage を作る。呼び出しは届いた順を待たずに並行して実行する。
func NewStage(handlers map[string]tools.Handler) *graph.Stage {
	if handlers == nil {
		handlers = map[string]tools.Handler{}
	}
	s := &toolCaller{
		upstream:   make(chan types.Event),
		results:    map[string]*types.ToolResultRecord{},
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		executor:   execution.New(handlers),
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
			event, ok := evt.(types.EventToolRequest)
			if !ok {
				continue
			}
			req := event.ToolRequest
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
		log.Printf("toolcaller: 開始 name=%s call_id=%s", req.Name, req.ToolCallID)
		started := time.Now()
		record := s.executor.Execute(s.ctx, req)
		log.Printf("toolcaller: 終了 name=%s call_id=%s elapsed=%s", req.Name, req.ToolCallID, time.Since(started).Round(time.Millisecond))
		result = &record
		s.mu.Lock()
		s.results[req.ToolCallID] = result
		s.mu.Unlock()
	}
	sent := *result // 送り直しのたびに別の値を渡し、受け手どうしで共有しない。
	s.emit(types.EventConversationCommitRequest{ConversationCommitRequest: types.ConversationCommitRequest{Role: types.RoleToolResult, GenerationID: sent.GenerationID, Source: sent.Name, ToolResult: &sent}})
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
