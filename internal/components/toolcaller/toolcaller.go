package toolcaller

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"

	"smart-speaker/internal/graph"
	"smart-speaker/internal/tools/switchbot"
	types "smart-speaker/internal/types"
)

type stage struct {
	upstream   chan types.Event
	downstream chan types.Event
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once
	lineWG     sync.WaitGroup

	switchClient  *switchbot.Client
	switchInitErr error
}

func NewStage() *graph.Stage {
	sbClient, sbErr := switchbot.NewFromEnv()
	s := &stage{
		upstream:      make(chan types.Event),
		downstream:    make(chan types.Event),
		switchClient:  sbClient,
		switchInitErr: sbErr,
	}
	return &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.Close,
	}
}

func (s *stage) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.ctx = ctx
	s.cancel = cancel
	s.lineWG.Add(1)
	go func() {
		defer s.lineWG.Done()
		defer close(s.downstream)
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
					resp := s.executeTool(req)
					outEvt := types.Event{Kind: types.EventToolResponse, Payload: resp}
					select {
					case <-s.ctx.Done():
						return
					case s.downstream <- outEvt:
					}
				default:
					// ignore
				}
			}
		}
	}()
}

func (s *stage) executeTool(req types.ToolRequest) types.ToolResponse {
	args := map[string]any{}
	if len(req.Arguments) > 0 {
		if err := json.Unmarshal(req.Arguments, &args); err != nil {
			log.Printf("toolcaller: arg parse error: %v", err)
			args = map[string]any{}
		}
	}
	var result map[string]any
	switch req.Name {
	case "switchbot_control_device":
		result = s.runSwitchBotTool(args)
	default:
		result = map[string]any{
			"error": "unknown function: " + req.Name,
		}
	}
	output, err := json.Marshal(result)
	if err != nil {
		log.Printf("toolcaller: result marshal error: %v", err)
		output = []byte(`{"error":"result encoding failed"}`)
	}
	return types.ToolResponse{ToolCallID: req.ToolCallID, Output: output}
}

func (s *stage) Close() error {
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.lineWG.Wait()
		close(s.upstream)
		log.Println("toolcaller: stage closed")
	})
	return nil
}

func (s *stage) runSwitchBotTool(args map[string]any) map[string]any {
	if s.switchClient == nil {
		if s.switchInitErr != nil {
			return map[string]any{"error": s.switchInitErr.Error()}
		}
		return map[string]any{"error": "SwitchBot が設定されていません"}
	}
	command := switchbot.Command{
		DeviceAlias: strings.TrimSpace(asString(args["device"])),
		DeviceID:    strings.TrimSpace(asString(args["device_id"])),
		Command:     strings.TrimSpace(asString(args["command"])),
		Parameter:   strings.TrimSpace(asString(args["parameter"])),
		CommandType: strings.TrimSpace(asString(args["command_type"])),
	}
	result, err := s.switchClient.Execute(s.ctx, command)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return result
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
