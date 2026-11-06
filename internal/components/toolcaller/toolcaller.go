package toolcaller

import (
	"context"
	"encoding/json"
	"log"
	"sync"

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

	tools     map[string]tools.Handler
	toolModes map[string]string
	node      *graph.Stage
	results   map[string]types.ToolResultRecord
	history   *toolhistory.Store
}

func NewStage(handlers map[string]tools.Handler, toolModes map[string]string) *graph.Stage {
	return NewStageWithHistory(handlers, toolModes, nil)
}

func NewStageWithHistory(handlers map[string]tools.Handler, toolModes map[string]string, history *toolhistory.Store) *graph.Stage {
	if handlers == nil {
		handlers = map[string]tools.Handler{}
	}
	if toolModes == nil {
		toolModes = map[string]string{}
	}
	s := &toolCaller{
		upstream:   make(chan types.Event),
		results:    map[string]types.ToolResultRecord{},
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		tools:      handlers,
		toolModes:  toolModes,
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
		if emitterTool, ok := tool.(tools.EventEmitterAware); ok {
			emitterTool.SetEventEmitter(s.emit)
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
			s.dispatchTool(req)
		}
	}()
}

func (s *toolCaller) dispatchTool(req types.ToolRequest) {
	result, exists := s.results[req.ToolCallID]
	if !exists {
		var entryID string
		if s.history != nil {
			entryID = s.history.Begin(req)
		}
		result = s.executeTool(req)
		if s.history != nil {
			s.history.Finish(entryID, result)
		}
		s.results[req.ToolCallID] = result
	}
	s.emit(types.Event{Kind: types.EventConversationCommitRequest, Payload: types.ConversationCommitRequest{Role: types.RoleToolResult, GenerationID: result.GenerationID, Source: result.Name, ToolResult: &result}})
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
