package toolcaller

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

// Tool は function calling から呼び出されるツールの抽象
type Tool interface {
	Name() string
	Run(args map[string]any) (map[string]any, error)
}

// ContextAwareTool は stage 管理の context を受け取れるツールが実装する
type ContextAwareTool interface {
	SetContext(ctx context.Context)
}

type toolCaller struct {
	upstream        chan types.Event
	downstream      chan types.Event
	ctx             context.Context
	cancel          context.CancelFunc
	once            sync.Once
	closerWaitGroup sync.WaitGroup
	taskGroup       sync.WaitGroup

	tools map[string]Tool
}

func NewStage(tools map[string]Tool) *graph.Stage {
	if tools == nil {
		tools = map[string]Tool{}
	}
	if _, ok := tools["web_fetch"]; !ok {
		tools["web_fetch"] = NewWebFetchTool()
	}
	if _, ok := tools["web_search"]; !ok {
		if ws := NewWebSearchTool(os.Getenv("OPENAI_API_KEY")); ws != nil {
			tools[ws.Name()] = ws
		}
	}
	s := &toolCaller{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		tools:      tools,
	}
	return &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.close,
	}
}

func (s *toolCaller) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.ctx = ctx
	s.cancel = cancel
	for _, tool := range s.tools {
		if ctxTool, ok := tool.(ContextAwareTool); ok {
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
			select {
			case <-s.ctx.Done():
				return
			case evt, ok := <-s.upstream:
				if !ok {
					return
				}
				switch evt.Kind {
				case types.EventToolRequest:
					req, ok := evt.Payload.(types.ToolRequest)
					if !ok {
						log.Printf("toolcaller: unexpected payload type %T", evt.Payload)
						continue
					}
					s.dispatchTool(req)
				default:
					// ignore
				}
			}
		}
	}()
}

func (s *toolCaller) dispatchTool(req types.ToolRequest) {
	s.taskGroup.Add(1)
	go func() {
		defer s.taskGroup.Done()
		resp := s.executeTool(req)
		select {
		case <-s.ctx.Done():
			return
		case s.downstream <- types.Event{Kind: types.EventToolResponse, Payload: resp}:
		}
	}()
}

func (s *toolCaller) executeTool(req types.ToolRequest) types.ToolResponse {
	args := map[string]any{}
	if len(req.Arguments) > 0 {
		if err := json.Unmarshal(req.Arguments, &args); err != nil {
			log.Printf("toolcaller: arg parse error: %v", err)
			args = map[string]any{}
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
	return types.ToolResponse{ToolCallID: req.ToolCallID, Output: output}
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
