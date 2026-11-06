package tools

import "testing"

func TestResponsePolicyCanDisableFollowupAndSetInstructions(t *testing.T) {
	if !ResponsePolicyFromDefinition(nil).OnSuccess {
		t.Fatal("未指定の再応答が無効です")
	}
	want := ResponsePolicy{OnSuccess: false, Instructions: "この操作の追加報告は不要です"}
	def := DefinitionWithResponsePolicy(map[string]any{"name": "update"}, want)
	if got := ResponsePolicyFromDefinition(def); got != want {
		t.Fatalf("設定が保持されません: %v", got)
	}
}
