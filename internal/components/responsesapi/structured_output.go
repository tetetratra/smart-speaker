package responsesapi

import (
	"encoding/json"
	"strings"
)

type structuredOutput struct {
	Speech      string `json:"speech"`
	Expectation int    `json:"expectation"`
}

func parseStructuredOutput(raw string) (structuredOutput, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return structuredOutput{}, false
	}
	var out structuredOutput
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return structuredOutput{}, false
	}
	return out, true
}

func clampExpectation(value int) int {
	if value < 0 {
		return 0
	}
	if value > 10 {
		return 10
	}
	return value
}
