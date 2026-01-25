package graph

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	types "smart-speaker/internal/types"
)

type EventDetailFormatter func(types.Event) string

func defaultEventDetailFormatters() map[types.EventKind]EventDetailFormatter {
	return map[types.EventKind]EventDetailFormatter{
		types.EventTextInput:         formatOutputLineDetail,
		types.EventRealtimeOutput:    formatOutputLineDetail,
		types.EventRealtimeAudio:     formatRealtimeAudioDetail,
		types.EventToolRequest:       formatToolRequestDetail,
		types.EventToolResponse:      formatToolResponseDetail,
		types.EventMCPCall:           formatMCPCallDetail,
		types.EventResponsesRequest:  formatResponsesRequestDetail,
		types.EventResponsesResponse: formatResponsesResponseDetail,
		types.EventRTCSignal:         formatRTCSignalDetail,
	}
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
	return fmt.Sprintf("%s --%s%s--> %s", fromName, eventName, detailPart, strings.Join(toNames, ", "))
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
	if line.ResponseID != "" {
		parts = append(parts, fmt.Sprintf("response_id=%s", line.ResponseID))
	}
	if line.Expectation != nil {
		parts = append(parts, fmt.Sprintf("expectation=%d", *line.Expectation))
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
	if req.Role != "" {
		parts = append(parts, fmt.Sprintf("role=%s", req.Role))
	}
	if len(req.Tools) > 0 {
		parts = append(parts, fmt.Sprintf("tools=%d", len(req.Tools)))
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
		fmt.Sprintf("mcp_calls=%d", len(resp.MCPCalls)),
		fmt.Sprintf("has_response=%t", resp.HasResponse),
	}
	if resp.ResponseID != "" {
		parts = append(parts, fmt.Sprintf("response_id=%s", resp.ResponseID))
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
	if req.ToolCallID != "" {
		parts = append(parts, fmt.Sprintf("tool_call_id=%s", req.ToolCallID))
	}
	if req.ResponseID != "" {
		parts = append(parts, fmt.Sprintf("response_id=%s", req.ResponseID))
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
	if resp.ToolCallID != "" {
		parts = append(parts, fmt.Sprintf("tool_call_id=%s", resp.ToolCallID))
	}
	return strings.Join(parts, ", ")
}

func formatMCPCallDetail(evt types.Event) string {
	call, ok := evt.Payload.(types.MCPCall)
	if !ok {
		return ""
	}
	args := string(call.Arguments)
	output := string(call.Output)
	parts := []string{
		fmt.Sprintf("name=%s", call.Name),
		fmt.Sprintf("arguments=%s", quoteText(args)),
		fmt.Sprintf("args_bytes=%d", len(call.Arguments)),
		fmt.Sprintf("output=%s", quoteText(output)),
		fmt.Sprintf("output_bytes=%d", len(call.Output)),
	}
	if call.ServerLabel != "" {
		parts = append(parts, fmt.Sprintf("server_label=%s", call.ServerLabel))
	}
	if call.CallID != "" {
		parts = append(parts, fmt.Sprintf("call_id=%s", call.CallID))
	}
	if call.ResponseID != "" {
		parts = append(parts, fmt.Sprintf("response_id=%s", call.ResponseID))
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

func formatRTCSignalDetail(evt types.Event) string {
	sig, ok := evt.Payload.(types.RTCSignal)
	if !ok {
		return ""
	}
	parts := []string{
		fmt.Sprintf("type=%s", sig.Type),
		fmt.Sprintf("sdp_chars=%d", utf8.RuneCountInString(sig.SDP)),
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
