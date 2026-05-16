package conversation

import (
	"strings"
	"testing"
	"time"
)

func TestDecideReaction(t *testing.T) {
	tests := []struct {
		name             string
		text             string
		source           string
		setup            func(*conversationCore, time.Time)
		wantLevel        reactionLevel
		wantReasonSubstr string
	}{
		{
			name:             "server-stt由来の短い感嘆をignoreにする",
			text:             "まじか",
			source:           "server-stt",
			wantLevel:        reactionIgnore,
			wantReasonSubstr: "short_exclamation",
		},
		{
			name:             "server-stt由来の短い独白をignoreにする",
			text:             "眠い",
			source:           "server-stt",
			wantLevel:        reactionIgnore,
			wantReasonSubstr: "short_self_talk",
		},
		{
			name:             "相談開始にも見える曖昧な発話をsilent_observeにする",
			text:             "明日どうしようかな",
			source:           "server-stt",
			wantLevel:        reactionSilentObserve,
			wantReasonSubstr: "ambiguous_thinking",
		},
		{
			name:             "質問をvoice_replyにする",
			text:             "今何時？",
			source:           "server-stt",
			wantLevel:        reactionVoiceReply,
			wantReasonSubstr: "question",
		},
		{
			name:             "依頼をvoice_replyにする",
			text:             "電気消して",
			source:           "server-stt",
			wantLevel:        reactionVoiceReply,
			wantReasonSubstr: "request",
		},
		{
			name:             "明確な呼びかけをvoice_replyにする",
			text:             "ねえ、聞いて",
			source:           "server-stt",
			wantLevel:        reactionVoiceReply,
			wantReasonSubstr: "addressed_to_assistant",
		},
		{
			name:   "会話継続中の追い質問をvoice_replyに寄せる",
			text:   "それで？",
			source: "server-stt",
			setup: func(core *conversationCore, now time.Time) {
				core.state.appendUtterance(&Utterance{
					ID:      "ai_1",
					Speaker: SpeakerAI,
					StartAt: now.Add(-10 * time.Second),
					Content: "先ほどの返答",
					Status:  UtterancePlayed,
				})
			},
			wantLevel:        reactionVoiceReply,
			wantReasonSubstr: "conversation_continuing",
		},
		{
			name:             "手入力は短い文でもvoice_replyにする",
			text:             "眠い",
			source:           "",
			wantLevel:        reactionVoiceReply,
			wantReasonSubstr: "manual_input",
		},
	}

	now := time.Date(2026, 5, 16, 6, 0, 0, 0, time.UTC)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core := newConversationCore()
			if tt.setup != nil {
				tt.setup(core, now)
			}
			got := core.decideReaction(reactionInput{Text: tt.text, Source: tt.source, Now: now})
			if got.Level != tt.wantLevel {
				t.Fatalf("level = %s, want %s; decision = %+v", got.Level, tt.wantLevel, got)
			}
			if !containsReason(got.Reasons, tt.wantReasonSubstr) {
				t.Fatalf("reasons = %v, want reason containing %q", got.Reasons, tt.wantReasonSubstr)
			}
		})
	}
}

func TestObservedMonologues(t *testing.T) {
	now := time.Date(2026, 5, 16, 6, 0, 0, 0, time.UTC)
	state := newSessionState()
	for i, text := range []string{"ひとつめ", "ふたつめ", "みっつめ", "よっつめ"} {
		state.addObservedMonologue(observedMonologue{Text: text, At: now.Add(time.Duration(i) * time.Second)})
	}
	if len(state.observedMonologues) != reactionSilentObserveMaxItems {
		t.Fatalf("observed len = %d, want %d", len(state.observedMonologues), reactionSilentObserveMaxItems)
	}
	if state.observedMonologues[0].Text != "ふたつめ" {
		t.Fatalf("first observed = %q, want ふたつめ", state.observedMonologues[0].Text)
	}

	recent := state.consumeRecentObservedMonologues(now.Add(10 * time.Second))
	if len(recent) != reactionSilentObserveMaxItems {
		t.Fatalf("recent len = %d, want %d", len(recent), reactionSilentObserveMaxItems)
	}
	if len(state.observedMonologues) != 0 {
		t.Fatalf("observed should be consumed, got %d", len(state.observedMonologues))
	}

	state.addObservedMonologue(observedMonologue{Text: "古い", At: now.Add(-2 * time.Minute)})
	if got := state.consumeRecentObservedMonologues(now); len(got) != 0 {
		t.Fatalf("expired observations len = %d, want 0", len(got))
	}
}

func containsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, want) {
			return true
		}
	}
	return false
}
