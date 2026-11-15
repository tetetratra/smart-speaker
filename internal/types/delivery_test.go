package types

import (
	"encoding/json"
	"testing"
)

func TestExternalInputDecidesRespondAndKeepsMetadata(t *testing.T) {
	quiet := ExternalInput(ConversationRecord{ID: "a", Role: RoleSystem, Text: "記録だけ"}, json.RawMessage(`{"delivery":"history_only","automation_id":"job"}`))
	if quiet.ModelShouldRespond || quiet.ModelAlreadyKnows || quiet.UserAlreadyKnows || !quiet.Record.External || quiet.Record.Role != RoleSystem {
		t.Fatal(quiet)
	}
	// delivery は制御情報なので残さず、識別情報だけを付随情報に残す。
	if quiet.Record.Metadata["context"] != `{"automation_id":"job"}` {
		t.Fatal(quiet.Record.Metadata)
	}
	respond := ExternalInput(ConversationRecord{ID: "b", Role: RoleBackground}, nil)
	if !respond.ModelShouldRespond || respond.Record.Role != RoleBackground || respond.Record.Metadata != nil {
		t.Fatal(respond)
	}
}

func TestOutputError(t *testing.T) {
	for output, want := range map[string]string{
		`{"error":"失敗"}`:       "失敗",
		`{"error":{"code":1}}`: `{"code":1}`,
		`{"error":null}`:       "",
		`{"ok":true}`:          "",
		`JSONではない`:             "",
	} {
		if got := OutputError(output); got != want {
			t.Fatalf("OutputError(%q) = %q, want %q", output, got, want)
		}
	}
}
