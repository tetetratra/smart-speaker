package responsesapi

import (
	"encoding/json"
	"strings"
)

type structuredOutput struct {
	Speech            string            `json:"speech"`
	PreSpeechPauseSec int               `json:"pre_speech_pause_sec"`
	PostSpeechWaitSec int               `json:"post_speech_wait_sec"`
	Chain             []structuredChain `json:"chain"`
}

type structuredChain struct {
	Speech            string `json:"speech"`
	PostSpeechWaitSec int    `json:"post_speech_wait_sec"`
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
