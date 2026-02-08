package responsesapi

import (
	"encoding/json"
	"strings"
)

type structuredOutput struct {
	Speech           string            `json:"speech"`
	Expectation      int               `json:"expectation"`
	ReplyExpectation int               `json:"reply_expectation"`
	Chain            []structuredChain `json:"chain"`
}

type structuredChain struct {
	Speech      string `json:"speech"`
	Expectation int    `json:"expectation"`
}

func parseStructuredOutput(raw string) (structuredOutput, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return structuredOutput{}, false
	}
	var out structuredOutput
	dec := json.NewDecoder(strings.NewReader(trimmed))
	if err := dec.Decode(&out); err != nil {
		return structuredOutput{}, false
	}
	return out, true
}

func clampExpectation(value int) int {
	if value < 0 {
		return 0
	}
	if value > 3 {
		return 3
	}
	return value
}
