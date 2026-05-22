package llm

import (
	"encoding/json"
	"strings"
)

func buildSystemPrompt(base string, toolSchemas []any) string {
	parts := []string{strings.TrimSpace(base), timelineJSONInstruction()}
	if len(toolSchemas) > 0 {
		if encoded, err := json.Marshal(toolSchemas); err == nil {
			parts = append(parts, "利用可能なlocal tool schema:\n"+string(encoded))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func timelineJSONInstruction() string {
	return strings.TrimSpace(`出力は必ずJSON objectのみで返してください。
root objectは {"items":[...]} の形です。
itemsの各要素は {"type":"speech","text":"..."}、{"type":"wait","sec":0.5}、{"type":"tool","name":"tool_name","args":{...}} のいずれかです。
speechとwaitは複数item出せます。
toolは1回の応答の末尾に最大1itemだけ出せます。
toolの後にspeech、wait、toolを続けてはいけません。
OpenAI function callingは使わず、tool呼び出しもitems内のtool objectとして表現してください。`)
}

func appendRetryInstruction(prompt string, err error, rawPreview string) string {
	if err == nil {
		return prompt
	}
	parts := []string{
		strings.TrimSpace(prompt),
		`直前の応答はJSON timeline契約違反でした。

次の応答では、通常の文章を絶対に出力しないでください。
出力してよいのは、{"items":[...]} 形式のJSON objectだけです。
Markdown、説明文、前置き、謝罪文、コードブロック、箇条書き、JSON配列、NDJSONは出力禁止です。

正しい出力例:
{"items":[{"type":"speech","text":"うん、聞いてるよ"},{"type":"wait","sec":1},{"type":"speech","text":"続けて、ね"}]}

悪い出力例:
うん続けて、聞いてるよ`,
	}
	if trimmed := strings.TrimSpace(rawPreview); trimmed != "" {
		parts = append(parts, "直前に出力した不正な内容:\n"+trimmed)
	}
	parts = append(parts, "違反理由:\n"+err.Error())
	return strings.Join(parts, "\n\n")
}
