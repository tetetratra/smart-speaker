package graph

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

type EventDetailFormatter func(types.Event) string

func defaultSuppressedForwardLogKinds() map[types.EventKind]struct{} {
	return map[types.EventKind]struct{}{
		types.EventRTCVADStatus:  {},
		types.EventRealtimeAudio: {},
	}
}

func defaultEventDetailFormatters() map[types.EventKind]EventDetailFormatter {
	return map[types.EventKind]EventDetailFormatter{
		types.EventHumanUtterance:            formatOutputLineDetail,
		types.EventSpeechEnd:                 formatSpeechEventDetail,
		types.EventRealtimeOutput:            formatOutputLineDetail,
		types.EventRealtimeAudio:             formatRealtimeAudioDetail,
		types.EventToolRequest:               formatToolRequestDetail,
		types.EventWhiteboardUpdate:          formatWhiteboardUpdateDetail,
		types.EventRTCSignal:                 formatRTCSignalDetail,
		types.EventConversationCommitRequest: formatCommitRequestDetail,
		types.EventLLMRequest:                formatLLMRequestDetail,
		types.EventTimelineItem:              formatTimelineItemDetail,
		types.EventPlayableSpeech:            formatPlayableSpeechDetail,
		types.EventScheduledItem:             formatScheduledItemDetail,
	}
}

func (g *Graph) shouldLogForwardEvent(kind types.EventKind) bool {
	if g == nil || g.suppressedForwardLogKinds == nil {
		return true
	}
	_, suppressed := g.suppressedForwardLogKinds[kind]
	return !suppressed
}

func (g *Graph) RegisterEventDetailFormatter(kind types.EventKind, fn EventDetailFormatter) {
	if g == nil {
		return
	}
	if g.eventDetailFormatters == nil {
		g.eventDetailFormatters = map[types.EventKind]EventDetailFormatter{}
	}
	g.eventDetailFormatters[kind] = fn
}

func (g *Graph) eventDetail(evt types.Event) (string, bool) {
	if g == nil || g.eventDetailFormatters == nil {
		return "", false
	}
	fn, ok := g.eventDetailFormatters[evt.Kind]
	if !ok || fn == nil {
		return "", false
	}
	return fn(evt), true
}

func (g *Graph) formatForwardLog(from *Stage, downstreams []*Stage, evt types.Event) string {
	eventName := evt.Kind.String()
	detail, hasDetail := g.eventDetail(evt)
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

func formatOutputLineDetail(evt types.Event) string {
	line, ok := evt.Payload.(types.OutputLine)
	if !ok {
		return ""
	}
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

func formatToolRequestDetail(evt types.Event) string {
	req, ok := evt.Payload.(types.ToolRequest)
	if !ok {
		return ""
	}
	args := string(req.Arguments)
	parts := []string{
		fmt.Sprintf("name=%s", req.Name),
		fmt.Sprintf("arguments=%s", quoteText(args)),
		fmt.Sprintf("args_bytes=%d", len(req.Arguments)),
		fmt.Sprintf("generation=%d", req.GenerationID),
	}
	return strings.Join(parts, ", ")
}

func formatRealtimeAudioDetail(evt types.Event) string {
	audio, ok := evt.Payload.(types.OutputAudio)
	if !ok {
		return ""
	}
	parts := []string{
		fmt.Sprintf("audio_bytes=%d", len(audio.Audio)),
	}
	if audio.Role != "" {
		parts = append(parts, fmt.Sprintf("role=%s", audio.Role))
	}
	return strings.Join(parts, ", ")
}

func formatSpeechEventDetail(evt types.Event) string {
	speech, ok := evt.Payload.(types.SpeechEvent)
	if !ok {
		return ""
	}
	parts := []string{
		fmt.Sprintf("at=%s", speech.CapturedAt.Format(time.RFC3339Nano)),
	}
	if speech.Source != "" {
		parts = append(parts, fmt.Sprintf("source=%s", speech.Source))
	}
	return strings.Join(parts, ", ")
}

func formatWhiteboardUpdateDetail(evt types.Event) string {
	update, ok := evt.Payload.(types.WhiteboardUpdate)
	if !ok {
		return ""
	}
	text := strings.TrimSpace(update.Content)
	return fmt.Sprintf("content=%s, chars=%d", quoteText(text), utf8.RuneCountInString(text))
}

func formatCommitRequestDetail(evt types.Event) string {
	req, ok := evt.Payload.(types.ConversationCommitRequest)
	if !ok {
		return ""
	}
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

func formatLLMRequestDetail(evt types.Event) string {
	req, ok := evt.Payload.(types.LLMRequest)
	if !ok {
		return ""
	}
	return fmt.Sprintf("role=%s, text=%s, generation=%d, request_id=%s", req.Role, quoteText(req.Text), req.GenerationID, req.RequestID)
}

func formatTimelineItemDetail(evt types.Event) string {
	item, ok := evt.Payload.(types.TimelineItem)
	if !ok {
		return ""
	}
	parts := []string{
		fmt.Sprintf("kind=%s", item.Kind),
		fmt.Sprintf("generation=%d", item.GenerationID),
	}
	if item.Text != "" {
		parts = append(parts, fmt.Sprintf("text=%s", quoteText(item.Text)))
	}
	if item.ToolName != "" {
		parts = append(parts, fmt.Sprintf("tool=%s", item.ToolName))
	}
	return strings.Join(parts, ", ")
}

func formatPlayableSpeechDetail(evt types.Event) string {
	speech, ok := evt.Payload.(types.PlayableSpeech)
	if !ok {
		return ""
	}
	return fmt.Sprintf("text=%s, generation=%d, duration=%.3f", quoteText(speech.Text), speech.GenerationID, speech.DurationSeconds)
}

func formatScheduledItemDetail(evt types.Event) string {
	switch payload := evt.Payload.(type) {
	case types.PlayableSpeech:
		return fmt.Sprintf("speech=%s, generation=%d", quoteText(payload.Text), payload.GenerationID)
	case types.ToolRequest:
		return fmt.Sprintf("tool=%s, generation=%d", payload.Name, payload.GenerationID)
	default:
		return ""
	}
}

func formatRTCSignalDetail(evt types.Event) string {
	sig, ok := evt.Payload.(types.RTCSignal)
	if !ok {
		return ""
	}
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
