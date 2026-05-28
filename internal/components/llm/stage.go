package llm

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/agentstatus"
	"github.com/tetetratra/smart-speaker/internal/states/conversationhistory"
	types "github.com/tetetratra/smart-speaker/internal/types"
)

const maxContractRetries = 10
const maxRawLinePreviewRunes = 400
const rawLinePreviewSuffix = "..."
const noResponseReasonIdleCandidate = "idle_candidate"
const noResponseReasonModel = "model_no_response"

type stage struct {
	upstream     chan types.Event
	downstream   chan types.Event
	client       responseClient
	history      historyReader
	agentStatus  agentStatusReader
	systemPrompt string
	once         sync.Once
	cancel       context.CancelFunc
}

func NewStage(cfg Config) (*graph.Stage, error) {
	client := cfg.Client
	var err error
	if client == nil {
		client, err = NewClient(cfg)
		if err != nil {
			return nil, err
		}
	}
	s := &stage{
		upstream:     make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		client:       client,
		history:      cfg.History,
		agentStatus:  cfg.AgentStatus,
		systemPrompt: buildSystemPrompt(cfg.Instructions, cfg.ToolSchemas),
	}
	return &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.close,
	}, nil
}

func (s *stage) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	go s.consume(ctx)
}

func (s *stage) consume(ctx context.Context) {
	defer close(s.downstream)
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-s.upstream:
			if !ok {
				return
			}
			if evt.Kind != types.EventLLMRequest {
				continue
			}
			req, ok := evt.Payload.(types.LLMRequest)
			if !ok {
				continue
			}
			go s.handleRequest(ctx, req)
		}
	}
}

func (s *stage) handleRequest(ctx context.Context, req types.LLMRequest) {
	items, err := s.requestTimeline(ctx, req)
	if err != nil {
		log.Printf("llm: drop response generation=%d request_id=%s err=%v", req.GenerationID, req.RequestID, err)
		return
	}
	for _, item := range items {
		select {
		case <-ctx.Done():
			return
		case s.downstream <- types.Event{Kind: types.EventTimelineItem, Payload: item}:
		}
	}
	select {
	case <-ctx.Done():
		return
	case s.downstream <- types.Event{
		Kind: types.EventAgentTimelineEnd,
		Payload: types.AgentTimelineEnd{
			GenerationID: req.GenerationID,
		},
	}:
	}
}

func (s *stage) requestTimeline(ctx context.Context, req types.LLMRequest) ([]types.TimelineItem, error) {
	var lastErr error
	basePrompt := s.systemPrompt
	noResponseReason := s.noResponseReason(req)
	if noResponseReason == noResponseReasonIdleCandidate {
		basePrompt = appendIdleFollowupInstruction(basePrompt)
	}
	systemPrompt := basePrompt
	for attempt := 1; attempt <= maxContractRetries; attempt++ {
		messages := s.messages(req)
		rawText, err := s.client.CreateResponse(ctx, messages, appendCurrentTimestamp(systemPrompt))
		if err != nil {
			return nil, err
		}
		items, err := parseTimelineJSON(rawText, req.GenerationID)
		if err == nil {
			if len(items) == 0 {
				reason := noResponseReason
				if reason == "" {
					reason = noResponseReasonModel
				}
				log.Printf(
					"llm: no response generation=%d request_id=%s reason=%s text=%q",
					req.GenerationID,
					req.RequestID,
					reason,
					strings.TrimSpace(req.Text),
				)
			}
			return items, nil
		}
		lastErr = err
		rawPreview := rawPreviewFromError(err)
		if rawPreview == "" {
			rawPreview = rawText
		}
		rawPreview = rawPreviewText(rawPreview)
		log.Printf("llm: invalid timeline response generation=%d request_id=%s attempt=%d/%d err=%v raw_preview=%q", req.GenerationID, req.RequestID, attempt, maxContractRetries, err, rawPreview)
		systemPrompt = appendRetryInstruction(basePrompt, err, rawPreview)
	}
	return nil, lastErr
}

func rawPreviewFromError(err error) string {
	var parseErr *timelineParseError
	if !errors.As(err, &parseErr) {
		return ""
	}
	return parseErr.RawPreview()
}

func rawPreviewText(rawText string) string {
	trimmed := strings.TrimSpace(rawText)
	runes := []rune(trimmed)
	if len(runes) <= maxRawLinePreviewRunes {
		return trimmed
	}
	return string(runes[:maxRawLinePreviewRunes]) + rawLinePreviewSuffix
}

func (s *stage) messages(req types.LLMRequest) []types.ChatMessage {
	if s.history != nil {
		if records := s.history.Snapshot(); len(records) > 0 {
			return conversationhistory.ToChatMessages(records)
		}
	}
	role := req.Role
	if role == "" {
		role = types.RoleUser
	}
	return []types.ChatMessage{{Role: role, Content: req.Text}}
}

func (s *stage) noResponseReason(req types.LLMRequest) string {
	if !s.isIdle() {
		return ""
	}
	if !isMonologueCandidate(strings.TrimSpace(req.Text)) {
		return ""
	}
	return noResponseReasonIdleCandidate
}

func (s *stage) isIdle() bool {
	if s.agentStatus == nil {
		return false
	}
	return s.agentStatus.Status() == agentstatus.StatusIdle
}

func isMonologueCandidate(text string) bool {
	if text == "" {
		return false
	}
	if strings.Contains(text, "？") || strings.Contains(text, "?") {
		return false
	}
	requestHints := []string{
		"して", "してくれ", "してほしい", "してください", "して下さい",
		"つけて", "消して", "教えて", "お願い", "頼む",
	}
	for _, hint := range requestHints {
		if strings.Contains(text, hint) {
			return false
		}
	}
	return true
}

func (s *stage) close() error {
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		close(s.upstream)
	})
	return nil
}
