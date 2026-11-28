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
	case types.EventConversationRecordAdded:
		return formatRecordDetail(e.ConversationRecord), true
	case types.EventDecision:
		return fmt.Sprintf("choice=%s, generation=%d, turn=%s", e.Choice, e.GenerationID, e.TurnID), true
	case types.EventBrainRequest:
		return fmt.Sprintf("generation=%d, request=%s", e.GenerationID, e.RequestID), true
	case types.EventResponseRequest:
		return fmt.Sprintf("generation=%d, new_generation=%t, notice=%s", e.GenerationID, e.NewGeneration, quoteText(e.Notice)), true
	case types.EventRTCSignal:
		return formatRTCSignalDetail(e.RTCSignal), true
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

// formatRecordDetail は記録の要約を返す。ツールの結果などは長いので、本文は先頭だけ出す。
func formatRecordDetail(record types.ConversationRecord) string {
	text := []rune(record.Text)
	head := string(text)
	if len(text) > recordDetailChars {
		head = string(text[:recordDetailChars]) + "…"
	}
	return fmt.Sprintf("role=%s, source=%s, text=%s, chars=%d", record.Role, record.Source, quoteText(head), len(text))
}

const recordDetailChars = 40

func formatSpeechEventDetail(speech types.SpeechEvent) string {
	parts := []string{
		fmt.Sprintf("at=%s", speech.CapturedAt.Format(time.RFC3339Nano)),
	}
	if speech.Source != "" {
		parts = append(parts, fmt.Sprintf("source=%s", speech.Source))
	}
	return strings.Join(parts, ", ")
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
