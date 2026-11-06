package realtime

import (
	"encoding/binary"
	"testing"
)

func TestNormalizePCM16PreservesDurationAndLevels(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		sampleRate, channels int
	}{
		{"WebRTCの48kHzステレオ", 48000, 2},
		{"WebRTCでモノラル化した48kHz", 48000, 1},
		{"変換済みの24kHz", 24000, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// 20msのPCMを作り、符号・音量・長さが変換後も維持されることを確認する。
			frames := tc.sampleRate / 50
			pcm := make([]byte, frames*tc.channels*2)
			for frame := 0; frame < frames; frame++ {
				for ch := 0; ch < tc.channels; ch++ {
					value := int16(20000)
					if tc.channels == 2 {
						value = []int16{30000, 10000}[ch]
					}
					if frame >= frames/2 {
						value = -value
					}
					binary.LittleEndian.PutUint16(pcm[(frame*tc.channels+ch)*2:], uint16(value))
				}
			}
			got := normalizePCM16(pcm, tc.sampleRate, tc.channels)
			if len(got) != 960 {
				t.Fatalf("20msの24kHzモノラルPCMは960バイトです: %d", len(got))
			}
			for frame := 0; frame < 480; frame++ {
				want := int16(20000)
				if frame >= 240 {
					want = -want
				}
				if value := int16(binary.LittleEndian.Uint16(got[frame*2:])); value != want {
					t.Fatalf("サンプル%dの音量または符号が違います: %d, 期待値: %d", frame, value, want)
				}
			}
		})
	}
}
