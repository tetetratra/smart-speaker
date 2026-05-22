package llm

import (
	"encoding/json"
	"strings"
)

func buildSystemPrompt(base string, toolSchemas []any) string {
	parts := []string{strings.TrimSpace(base), ndjsonInstruction()}
	if len(toolSchemas) > 0 {
		if encoded, err := json.Marshal(toolSchemas); err == nil {
			parts = append(parts, "利用可能なlocal tool schema:\n"+string(encoded))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func ndjsonInstruction() string {
	return strings.TrimSpace(`出力は必ずNDJSONのみで返してください。
各行は {"type":"speech","text":"..."}、{"type":"wait","sec":0.5}、{"type":"tool","name":"tool_name","args":{...}} のいずれかです。
speechとwaitは複数行出せます。
toolは1回の応答の末尾に最大1行だけ出せます。
toolの後にspeech、wait、toolを続けてはいけません。
OpenAI function callingは使わず、tool呼び出しもNDJSON行として表現してください。`)
}

func appendRetryInstruction(prompt string, err error) string {
	if err == nil {
		return prompt
	}
	return strings.TrimSpace(prompt) + "\n\n直前の応答はNDJSON契約違反でした。次の応答では契約を必ず守ってください。違反理由: " + err.Error()
}
