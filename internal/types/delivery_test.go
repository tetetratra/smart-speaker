package types

import (
	"encoding/json"
	"testing"
)

func TestExternalInputDecidesModelHandlingAndKeepsMetadata(t *testing.T) {
	quiet := ExternalInput(ConversationRecord{ID: "a", Text: "記録だけ"}, json.RawMessage(`{"delivery":"history_only","automation_id":"job"}`))
	if quiet.ModelHandling != ModelHandlingAddToContext || !quiet.Record.External || quiet.Record.Role != RoleSystem {
		t.Fatal(quiet)
	}
	// delivery は制御情報なので残さず、識別情報だけを付随情報に残す。
	if quiet.Record.Metadata["context"] != `{"automation_id":"job"}` {
		t.Fatal(quiet.Record.Metadata)
	}
	respond := ExternalInput(ConversationRecord{ID: "b", Role: RoleToolResult}, nil)
	if respond.ModelHandling != ModelHandlingAddAndRespond || respond.Record.Role != RoleToolResult || respond.Record.Metadata != nil {
		t.Fatal(respond)
	}
}
