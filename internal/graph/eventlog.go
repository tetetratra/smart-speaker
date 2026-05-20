package graph

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	types "smart-speaker/internal/types"
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
		types.EventHumanUtterance:       formatOutputLineDetail,
		types.EventSpeechEnd:            formatSpeechEventDetail,
		types.EventRealtimeOutput:       formatOutputLineDetail,
		types.EventRealtimeAudio:        formatRealtimeAudioDetail,
		types.EventToolRequest:          formatToolRequestDetail,
		types.EventToolResponse:         formatToolResponseDetail,
		types.EventResponsesRequest:     formatResponsesRequestDetail,
		types.EventResponsesResponse:    formatResponsesResponseDetail,
		types.EventResponsesStreamChunk: formatResponsesStreamChunkDetail,
		types.EventWhiteboardUpdate:     formatWhiteboardUpdateDetail,
		types.EventTTSCancel:            formatTTSCancelDetail,
		types.EventRTCSignal:            formatRTCSignalDetail,
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

func formatResponsesRequestDetail(evt types.Event) string {
	req, ok := evt.Payload.(types.ResponsesRequest)
	if !ok {
		return ""
	}
	parts := []string{
		fmt.Sprintf("text=%s", quoteText(req.Text)),
		fmt.Sprintf("chars=%d", utf8.RuneCountInString(req.Text)),
	}
	if len(req.Messages) > 0 {
		parts = append(parts, fmt.Sprintf("messages=%d", len(req.Messages)))
	}
	if req.Role != "" {
		parts = append(parts, fmt.Sprintf("role=%s", req.Role))
	}
	if len(req.Tools) > 0 {
		parts = append(parts, fmt.Sprintf("tools=%d", len(req.Tools)))
	}
	if req.RequestID != "" {
		parts = append(parts, fmt.Sprintf("request_id=%s", req.RequestID))
	}
	return strings.Join(parts, ", ")
}

func formatResponsesResponseDetail(evt types.Event) string {
	resp, ok := evt.Payload.(types.ResponsesResponse)
	if !ok {
		return ""
	}
	parts := []string{
		fmt.Sprintf("text=%s", quoteText(resp.Text)),
		fmt.Sprintf("chars=%d", utf8.RuneCountInString(resp.Text)),
		fmt.Sprintf("tool_calls=%d", len(resp.ToolCalls)),
		fmt.Sprintf("has_response=%t", resp.HasResponse),
	}
	if resp.RequestID != "" {
		parts = append(parts, fmt.Sprintf("request_id=%s", resp.RequestID))
	}
	return strings.Join(parts, ", ")
}

func formatResponsesStreamChunkDetail(evt types.Event) string {
	chunk, ok := evt.Payload.(types.ResponsesStreamChunk)
	if !ok {
		return ""
	}
	parts := []string{
		fmt.Sprintf("line=%s", quoteText(chunk.Line)),
		fmt.Sprintf("chars=%d", utf8.RuneCountInString(chunk.Line)),
		fmt.Sprintf("done=%t", chunk.Done),
	}
	if chunk.RequestID != "" {
		parts = append(parts, fmt.Sprintf("request_id=%s", chunk.RequestID))
	}
	if chunk.ResponseID != "" {
		parts = append(parts, fmt.Sprintf("response_id=%s", chunk.ResponseID))
	}
	if chunk.Err != "" {
		parts = append(parts, fmt.Sprintf("err=%s", quoteText(chunk.Err)))
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
	}
	return strings.Join(parts, ", ")
}

func formatToolResponseDetail(evt types.Event) string {
	resp, ok := evt.Payload.(types.ToolResponse)
	if !ok {
		return ""
	}
	output := string(resp.Output)
	parts := []string{
		fmt.Sprintf("output=%s", quoteText(output)),
		fmt.Sprintf("output_bytes=%d", len(resp.Output)),
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

func formatTTSCancelDetail(evt types.Event) string {
	cancel, ok := evt.Payload.(types.TTSCancel)
	if !ok {
		return ""
	}
	if cancel.ResponseID == "" {
		return ""
	}
	return fmt.Sprintf("response_id=%s", cancel.ResponseID)
}

func formatWhiteboardUpdateDetail(evt types.Event) string {
	update, ok := evt.Payload.(types.WhiteboardUpdate)
	if !ok {
		return ""
	}
	text := strings.TrimSpace(update.Content)
	return fmt.Sprintf("content=%s, chars=%d", quoteText(text), utf8.RuneCountInString(text))
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
