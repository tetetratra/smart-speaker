package graph

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func defaultSuppressedForwardLogTypes() map[reflect.Type]struct{} {
	suppressed := map[reflect.Type]struct{}{}
	for _, e := range []types.Event{types.EventAudioFrame{}, types.EventAudioSent{}, types.EventResponseText{}, types.EventRTCPeerAudioFrame{}} {
		suppressed[reflect.TypeOf(e)] = struct{}{}
	}
	return suppressed
}

func (g *Graph) shouldLogForwardEvent(eventType reflect.Type) bool {
	if g == nil || g.suppressedForwardLogTypes == nil {
		return true
	}
	_, suppressed := g.suppressedForwardLogTypes[eventType]
	return !suppressed
}

// eventDetail は、転送ログでイベント名の後ろに添える要約を返す。要約しないイベントは false。
func eventDetail(evt types.Event) (string, bool) {
	switch e := evt.(type) {
	case types.EventSpeechEnd:
		return formatSpeechEventDetail(e.SpeechEvent), true
	case types.EventRealtimeOutput:
		return formatOutputLineDetail(e.OutputLine), true
	case types.EventToolRequest:
		return formatToolRequestDetail(e.ToolRequest), true
	case types.EventRTCSignal:
		return formatRTCSignalDetail(e.RTCSignal), true
	case types.EventConversationCommitRequest:
		return formatCommitRequestDetail(e.ConversationCommitRequest), true
	case types.EventLLMRequest:
		return formatLLMRequestDetail(e.LLMRequest), true
	case types.EventTimelineItem:
		return formatTimelineItemDetail(e.TimelineItem), true
	case types.EventPlayableSpeech:
		return formatPlayableSpeechDetail(e.PlayableSpeech), true
	case types.EventAgentTimelineEnd:
		return fmt.Sprintf("generation=%d", e.GenerationID), true
	}
	return "", false
}

func (g *Graph) formatForwardLog(from *Stage, downstreams []*Stage, evt types.Event) string {
	eventName := types.EventName(evt)
	detail, hasDetail := eventDetail(evt)
	detailPart := ""
	if hasDetail {
		detailPart = "{" + detail + "}"
	}
	fromName := stageName(from)
	toNames := stageNames(downstreams)
	return fmt.Sprintf("%s -- %s%s --> %s", fromName, eventName, detailPart, strings.Join(toNames, ","))
}

func stageName(stage *Stage) string {
	if stage == nil || stage.Name == "" {
		return "unknown"
	}
	return stage.Name
}

func stageNames(stages []*Stage) []string {
	names := make([]string, 0, len(stages))
	for _, st := range stages {
		names = append(names, stageName(st))
	}
	return names
}

func formatOutputLineDetail(line types.OutputLine) string {
	parts := []string{
		fmt.Sprintf("text=%s", quoteText(line.Text)),
		fmt.Sprintf("chars=%d", utf8.RuneCountInString(line.Text)),
		fmt.Sprintf("final=%t", line.Final),
	}
	if line.Role != "" {
		parts = append(parts, fmt.Sprintf("role=%s", line.Role))
	}
	if line.Source != "" {
		parts = append(parts, fmt.Sprintf("source=%s", line.Source))
	}
	return strings.Join(parts, ", ")
}

func formatToolRequestDetail(req types.ToolRequest) string {
	args := string(req.Arguments)
	parts := []string{
		fmt.Sprintf("name=%s", req.Name),
		fmt.Sprintf("arguments=%s", quoteText(args)),
		fmt.Sprintf("args_bytes=%d", len(req.Arguments)),
		fmt.Sprintf("generation=%d", req.GenerationID),
	}
	return strings.Join(parts, ", ")
}

func formatSpeechEventDetail(speech types.SpeechEvent) string {
	parts := []string{
		fmt.Sprintf("at=%s", speech.CapturedAt.Format(time.RFC3339Nano)),
	}
	if speech.Source != "" {
		parts = append(parts, fmt.Sprintf("source=%s", speech.Source))
	}
	return strings.Join(parts, ", ")
}

func formatCommitRequestDetail(req types.ConversationCommitRequest) string {
	parts := []string{
		fmt.Sprintf("role=%s", req.Role),
		fmt.Sprintf("text=%s", quoteText(req.Text)),
		fmt.Sprintf("generation=%d", req.GenerationID),
	}
	if req.ToolResult != nil {
		parts = append(parts, fmt.Sprintf("tool=%s", req.ToolResult.Name))
	}
	return strings.Join(parts, ", ")
}

func formatLLMRequestDetail(req types.LLMRequest) string {
	return fmt.Sprintf("role=%s, text=%s, generation=%d, request_id=%s", req.Role, quoteText(req.Text), req.GenerationID, req.RequestID)
}

func formatTimelineItemDetail(item types.TimelineItem) string {
	parts := []string{
		fmt.Sprintf("kind=%s", item.Kind),
		fmt.Sprintf("generation=%d", item.GenerationID),
	}
	if item.Text != "" {
		parts = append(parts, fmt.Sprintf("text=%s", quoteText(item.Text)))
	}
	return strings.Join(parts, ", ")
}

func formatPlayableSpeechDetail(speech types.PlayableSpeech) string {
	return fmt.Sprintf("text=%s, generation=%d, duration=%.3f", quoteText(speech.Text), speech.GenerationID, speech.DurationSeconds)
}

func formatRTCSignalDetail(sig types.RTCSignal) string {
	parts := []string{
		fmt.Sprintf("type=%s", sig.Type),
		fmt.Sprintf("sdp_chars=%d", utf8.RuneCountInString(sig.SDP)),
	}
	if sig.ClientID != "" {
		parts = append(parts, fmt.Sprintf("client_id=%s", sig.ClientID))
	}
	if sig.Candidate != nil {
		parts = append(parts, fmt.Sprintf("candidate_chars=%d", utf8.RuneCountInString(sig.Candidate.Candidate)))
		if sig.Candidate.SDPMid != nil {
			parts = append(parts, fmt.Sprintf("sdp_mid=%s", *sig.Candidate.SDPMid))
		}
		if sig.Candidate.SDPMLineIndex != nil {
			parts = append(parts, fmt.Sprintf("sdp_mline_index=%d", *sig.Candidate.SDPMLineIndex))
		}
	}
	return strings.Join(parts, ", ")
}

func quoteText(text string) string {
	return strconv.Quote(text)
}
