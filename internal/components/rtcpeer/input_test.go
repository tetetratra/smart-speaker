package rtcpeer

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestDownmixToMono(t *testing.T) {
	t.Run("copies mono input", func(t *testing.T) {
		in := []int16{1, -2, 3}
		got := downmixToMono(in, 1)

		if !equalInt16(got, in) {
			t.Fatalf("downmixToMono() = %#v, want %#v", got, in)
		}
		got[0] = 99
		if in[0] == 99 {
			t.Fatal("expected mono input to be copied")
		}
	})

	t.Run("averages stereo frames", func(t *testing.T) {
		got := downmixToMono([]int16{10, 30, -20, 10}, 2)
		want := []int16{20, -5}

		if !equalInt16(got, want) {
			t.Fatalf("downmixToMono() = %#v, want %#v", got, want)
		}
	})

	t.Run("returns nil for incomplete multichannel frame", func(t *testing.T) {
		if got := downmixToMono([]int16{1}, 2); got != nil {
			t.Fatalf("downmixToMono() = %#v, want nil", got)
		}
	})
}

func TestInt16ToBytes(t *testing.T) {
	got := int16ToBytes([]int16{1, -2, 300})
	want := new(bytes.Buffer)
	for _, sample := range []int16{1, -2, 300} {
		if err := binary.Write(want, binary.LittleEndian, sample); err != nil {
			t.Fatal(err)
		}
	}

	if !bytes.Equal(got, want.Bytes()) {
		t.Fatalf("int16ToBytes() = %v, want %v", got, want.Bytes())
	}
	if got := int16ToBytes(nil); got != nil {
		t.Fatalf("int16ToBytes(nil) = %#v, want nil", got)
	}
}

func TestPacketDurationMs(t *testing.T) {
	tests := []struct {
		name       string
		samples    int
		sampleRate int
		want       int
	}{
		{name: "20ms at 48kHz", samples: 960, sampleRate: 48000, want: 20},
		{name: "rounds tiny positive frame up to 20ms", samples: 1, sampleRate: 48000, want: 20},
		{name: "zero sample count", samples: 0, sampleRate: 48000, want: 0},
		{name: "zero sample rate", samples: 960, sampleRate: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := packetDurationMs(tt.samples, tt.sampleRate); got != tt.want {
				t.Fatalf("packetDurationMs() = %d, want %d", got, tt.want)
			}
		})
	}
}

func equalInt16(a, b []int16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
