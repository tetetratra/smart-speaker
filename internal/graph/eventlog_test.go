package graph

import (
	"reflect"
	"strings"
	"testing"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestShouldLogForwardEvent(t *testing.T) {
	g := New()

	if g.shouldLogForwardEvent(reflect.TypeOf(types.EventAudioFrame{})) {
		t.Fatalf("EventAudioFrame should be suppressed")
	}
	if g.shouldLogForwardEvent(reflect.TypeOf(types.EventRTCPeerAudioFrame{})) {
		t.Fatalf("EventRTCPeerAudioFrame should be suppressed")
	}
	if !g.shouldLogForwardEvent(reflect.TypeOf(types.EventConversationRecordAdded{})) {
		t.Fatalf("EventConversationRecordAdded should still be logged")
	}
}

// ツールの結果などの長い記録は、本文の先頭だけをログに出す。
func TestFormatRecordDetailTruncatesLongText(t *testing.T) {
	short := formatRecordDetail(types.ConversationRecord{Role: types.RoleUser, Source: "realtime", Text: "おはよう"})
	if short != `role=user, source=realtime, text="おはよう", chars=4` {
		t.Fatal(short)
	}
	long := formatRecordDetail(types.ConversationRecord{Role: types.RoleTool, Source: "web_search", Text: strings.Repeat("あ", 100)})
	if long != `role=tool, source=web_search, text="`+strings.Repeat("あ", recordDetailChars)+`…", chars=100` {
		t.Fatal(long)
	}
}
