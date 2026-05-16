package conversation

import (
	"strings"
	"time"
	"unicode/utf8"

	types "smart-speaker/internal/types"
)

const (
	reactionSilentObserveMaxItems = 3
	reactionSilentObserveTTL      = 60 * time.Second
	reactionContinuingWindow      = 90 * time.Second
)

type reactionLevel string

const (
	reactionIgnore        reactionLevel = "ignore"
	reactionSilentObserve reactionLevel = "silent_observe"
	reactionVoiceReply    reactionLevel = "voice_reply"
)

type reactionInput struct {
	Text   string
	Source string
	Now    time.Time
}

type reactionDecision struct {
	Level   reactionLevel
	Text    string
	Source  string
	Reasons []string
	Score   int
	At      time.Time
}

type observedMonologue struct {
	Text    string
	Source  string
	Reasons []string
	Score   int
	At      time.Time
}

func (c *conversationCore) decideReaction(input reactionInput) reactionDecision {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	text := strings.TrimSpace(input.Text)
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = "manual"
	}

	decision := reactionDecision{
		Level:  reactionVoiceReply,
		Text:   text,
		Source: source,
		At:     now,
	}
	if !isMonologueGateSource(source) {
		decision.Reasons = append(decision.Reasons, "manual_input")
		decision.Score = 100
		return decision
	}

	score := 0
	var reasons []string
	if c.isConversationContinuing(now) {
		score += 4
		reasons = append(reasons, "conversation_continuing")
	}
	if hasQuestionSignal(text) {
		score += 4
		reasons = append(reasons, "question")
	}
	if hasRequestSignal(text) {
		score += 4
		reasons = append(reasons, "request")
	}
	if hasAddressSignal(text) {
		score += 3
		reasons = append(reasons, "addressed_to_assistant")
	}
	if hasClearIntentSignal(text) {
		score += 3
		reasons = append(reasons, "clear_intent")
	}
	if isShortExclamation(text) {
		score -= 5
		reasons = append(reasons, "short_exclamation")
	}
	if isShortSelfTalk(text) {
		score -= 3
		reasons = append(reasons, "short_self_talk")
	}
	if hasSelfContainedEnding(text) {
		score -= 2
		reasons = append(reasons, "self_contained")
	}
	if hasAmbiguousThinkingSignal(text) {
		reasons = append(reasons, "ambiguous_thinking")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "no_clear_signal")
	}

	level := reactionSilentObserve
	switch {
	case score >= 3:
		level = reactionVoiceReply
	case score <= -4:
		level = reactionIgnore
	default:
		level = reactionSilentObserve
	}

	decision.Level = level
	decision.Reasons = reasons
	decision.Score = score
	return decision
}

func isMonologueGateSource(source string) bool {
	return strings.TrimSpace(source) == "server-stt"
}

func (c *conversationCore) isConversationContinuing(now time.Time) bool {
	if c == nil || c.state == nil {
		return false
	}
	if c.state.current != nil {
		return true
	}
	if c.state.pendingRequestID != "" || c.state.pendingRequestStreaming || len(c.state.pendingTimeline) > c.state.pendingTimelineIdx {
		return true
	}
	for i := len(c.state.conversation) - 1; i >= 0; i-- {
		utt := c.state.conversation[i]
		if utt == nil {
			continue
		}
		if utt.Speaker != SpeakerAI {
			return false
		}
		if utt.StartAt.IsZero() {
			return true
		}
		return now.Sub(utt.StartAt) <= reactionContinuingWindow
	}
	return false
}

func (s *sessionState) addObservedMonologue(item observedMonologue) {
	if s == nil || strings.TrimSpace(item.Text) == "" {
		return
	}
	if item.At.IsZero() {
		item.At = time.Now()
	}
	s.observedMonologues = append(s.observedMonologues, item)
	if len(s.observedMonologues) > reactionSilentObserveMaxItems {
		s.observedMonologues = s.observedMonologues[len(s.observedMonologues)-reactionSilentObserveMaxItems:]
	}
}

func (s *sessionState) consumeRecentObservedMonologues(now time.Time) []observedMonologue {
	if s == nil || len(s.observedMonologues) == 0 {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	var recent []observedMonologue
	for _, item := range s.observedMonologues {
		if item.At.IsZero() || now.Sub(item.At) <= reactionSilentObserveTTL {
			recent = append(recent, item)
		}
	}
	s.observedMonologues = nil
	return recent
}

func injectObservedMonologueContext(messages []types.ChatMessage, observations []observedMonologue) []types.ChatMessage {
	if len(messages) == 0 || len(observations) == 0 {
		return messages
	}
	lines := make([]string, 0, len(observations)+1)
	lines = append(lines, "直前に、ユーザーは音声応答を返さず観察した発話として次の内容を話していました。必要な場合だけ現在の会話の文脈として扱ってください。")
	for _, item := range observations {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		lines = append(lines, "- "+text)
	}
	if len(lines) == 1 {
		return messages
	}
	context := types.ChatMessage{Role: "system", Content: strings.Join(lines, "\n")}
	out := make([]types.ChatMessage, 0, len(messages)+1)
	if len(messages) == 1 {
		out = append(out, context)
		out = append(out, messages...)
		return out
	}
	out = append(out, messages[:len(messages)-1]...)
	out = append(out, context)
	out = append(out, messages[len(messages)-1])
	return out
}

func reactionLogRecord(decision reactionDecision) logRecord {
	score := decision.Score
	passed := decision.Level == reactionVoiceReply
	return logRecord{
		Speaker:         "reaction_gate",
		Text:            decision.Text,
		Source:          decision.Source,
		ReactionLevel:   string(decision.Level),
		ReactionReasons: append([]string(nil), decision.Reasons...),
		ReactionScore:   &score,
		PassedToLLM:     &passed,
	}
}

func humanLogRecord(text string, source string, decision reactionDecision) logRecord {
	score := decision.Score
	passed := true
	return logRecord{
		Speaker:         "human",
		Text:            text,
		Source:          source,
		ReactionLevel:   string(decision.Level),
		ReactionReasons: append([]string(nil), decision.Reasons...),
		ReactionScore:   &score,
		PassedToLLM:     &passed,
	}
}

func hasQuestionSignal(text string) bool {
	return strings.Contains(text, "?") || strings.Contains(text, "？") ||
		strings.Contains(text, "教えて") || strings.Contains(text, "どう思う") ||
		strings.Contains(text, "どうしたら") || strings.Contains(text, "なに") ||
		strings.Contains(text, "何") || strings.Contains(text, "いつ") ||
		strings.Contains(text, "どこ") || strings.Contains(text, "誰") ||
		strings.Contains(text, "なぜ") || strings.Contains(text, "なんで")
}

func hasRequestSignal(text string) bool {
	keywords := []string{"して", "してほしい", "してくれる", "お願い", "頼む", "消して", "つけて", "上げて", "下げて", "調べて", "覚えて", "記録して", "予定", "タイマー"}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func hasAddressSignal(text string) bool {
	prefixes := []string{"ねえ", "ねぇ", "おーい", "ちょっと", "すみません", "聞いて"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func hasClearIntentSignal(text string) bool {
	keywords := []string{"天気", "時間", "何時", "予定", "電気", "音量", "ニュース", "リマインド"}
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func isShortExclamation(text string) bool {
	normalized := strings.Trim(text, " 　。、.!！?？")
	if utf8.RuneCountInString(normalized) > 6 {
		return false
	}
	phrases := []string{"あ", "あー", "ああ", "え", "えー", "うわ", "わ", "わー", "やば", "やばい", "まじ", "まじか", "へえ", "ふーん", "おっと"}
	for _, phrase := range phrases {
		if normalized == phrase {
			return true
		}
	}
	return false
}

func isShortSelfTalk(text string) bool {
	return utf8.RuneCountInString(text) <= 8 && !hasQuestionSignal(text) && !hasRequestSignal(text) && !hasAddressSignal(text)
}

func hasSelfContainedEnding(text string) bool {
	suffixes := []string{"だな", "だね", "なあ", "かな", "かも", "っぽい", "疲れた", "眠い", "寒い", "暑い"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(text, suffix) {
			return true
		}
	}
	return false
}

func hasAmbiguousThinkingSignal(text string) bool {
	return strings.Contains(text, "どうしよう") || strings.Contains(text, "かな") || strings.Contains(text, "困った")
}
