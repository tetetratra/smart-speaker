package utterancebuffer

import "strings"

type buffer struct {
	parts []string
}

func (b *buffer) append(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	b.parts = append(b.parts, text)
}

func (b *buffer) text() string {
	return strings.TrimSpace(strings.Join(b.parts, " "))
}

func (b *buffer) reset() {
	b.parts = nil
}

func (b *buffer) empty() bool {
	return strings.TrimSpace(b.text()) == ""
}
