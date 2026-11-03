package speechsegment

import (
	"strings"
	"testing"
)

func TestSplit(t *testing.T) {
	for _, tt := range []struct {
		input      string
		flush      bool
		part, rest string
	}{
		{"こんにちは。続き", false, "こんにちは。", "続き"}, {"短い途中", false, "", "短い途中"}, {"残り", true, "残り", ""},
		{strings.Repeat("あ", 40) + "、" + strings.Repeat("い", 40), false, strings.Repeat("あ", 40) + "、", strings.Repeat("い", 40)},
		{strings.Repeat("あ", 81), false, strings.Repeat("あ", 80), "あ"},
	} {
		p, r := Split(tt.input, tt.flush)
		if p != tt.part || r != tt.rest {
			t.Fatalf("%q: %q %q", tt.input, p, r)
		}
	}
}
