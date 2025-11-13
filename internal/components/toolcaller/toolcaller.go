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

type Stage struct {
	upstream   chan interface{}
	downstream chan interface{}
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once

	switchClient  *switchbot.Client
	switchInitErr error
}

func NewStage() *Stage {
	ctx, cancel := context.WithCancel(context.Background())
	sbClient, sbErr := switchbot.NewFromEnv()
	s := &Stage{
		upstream:      make(chan interface{}),
		downstream:    make(chan interface{}),
		ctx:           ctx,
		cancel:        cancel,
		switchClient:  sbClient,
		switchInitErr: sbErr,
	}
	go s.run()
	return s
}

func (s *Stage) run() {
	defer close(s.downstream)
	for {
		select {
		case <-s.ctx.Done():
			return
		case data, ok := <-s.upstream:
			if !ok {
				return
			}
			evt, ok := data.(types.Event)
			if !ok {
				log.Printf("toolcaller: unexpected upstream type %T", data)
				continue
			}
			if evt.Kind != types.EventToolRequest {
				continue
			}
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
		}
	}
}

func (s *Stage) executeTool(req types.ToolRequest) types.ToolResponse {
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

func (s *Stage) Upstream() chan<- interface{} { return s.upstream }

func (s *Stage) Downstream() <-chan interface{} { return s.downstream }

func (s *Stage) Close() error {
	s.once.Do(func() {
		s.cancel()
		close(s.upstream)
	})
	return nil
}

func (s *Stage) runSwitchBotTool(args map[string]any) map[string]any {
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

var _ graph.Stage = (*Stage)(nil)
