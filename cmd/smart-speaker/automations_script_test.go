package main

import (
	"context"
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/automations"
	"github.com/tetetratra/smart-speaker/internal/states/toolhistory"
	"github.com/tetetratra/smart-speaker/internal/tools"
	"github.com/tetetratra/smart-speaker/internal/types"
)

type fakeTool struct {
	name string
	args []map[string]any
}

func (f *fakeTool) Name() string { return f.name }
func (f *fakeTool) Run(args map[string]any) (map[string]any, error) {
	f.args = append(f.args, args)
	return map[string]any{"temperature": 30}, nil
}

func TestAutomationRunnerRemindAndToolCall(t *testing.T) {
	scheduled := make(chan types.ScheduledPrompt, 10)
	direct := make(chan types.DirectToolCall, 10)
	tool := &fakeTool{name: "get_temperature"}
	runner := &automationRunner{scheduled: scheduled, direct: direct, history: toolhistory.NewStore(10), handlers: map[string]tools.Handler{tool.name: tool}}
	src := `
const r = toolcall("get_temperature", {device: "Hub2"});
if (r.temperature > 28) { remind("暑いと伝えて"); remind("水分補給も促して"); }
`
	if err := runner.run(context.Background(), automations.Delivery{ID: "job1", Name: "室温", Script: src}); err != nil {
		t.Fatal(err)
	}
	if len(tool.args) != 1 || tool.args[0]["device"] != "Hub2" {
		t.Fatal(tool.args)
	}
	call := <-direct
	if call.Source != "automation/job1" || call.Request.Name != "get_temperature" || string(call.Result.Output) != `{"temperature":30}` {
		t.Fatal(call)
	}
	first, second := <-scheduled, <-scheduled
	if first.ID == second.ID || first.Instruction != "暑いと伝えて" || !first.Speak || first.Source != "automation/job1" || second.Source != "automation/job1" {
		t.Fatal(first, second)
	}
	if entries := runner.history.Snapshot(); len(entries) != 1 {
		t.Fatal(entries)
	}
}

func TestAutomationRunnerRejectsUserOnlyAndUnknownTools(t *testing.T) {
	runner := &automationRunner{direct: make(chan types.DirectToolCall, 1), handlers: map[string]tools.Handler{}}
	for name, want := range map[string]string{"add_memory": "了承", "nothing": "ツールがありません"} {
		err := runner.run(context.Background(), automations.Delivery{ID: "job1", Script: `toolcall("` + name + `", {})`})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
