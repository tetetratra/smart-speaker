package openaistt

import "testing"

func TestNormalizePCM16Downsamples48kMonoTo24kMono(t *testing.T) {
	in := encodePCM16([]int16{100, 200, 300, 400})

	got := normalizePCM16(in, 48000, 1)
	samples := decodePCM16(got)
	want := []int16{100, 300}
	if len(samples) != len(want) {
		t.Fatalf("len = %d, want %d (%#v)", len(samples), len(want), samples)
	}
	for i := range want {
		if samples[i] != want[i] {
			t.Fatalf("sample[%d] = %d, want %d (%#v)", i, samples[i], want[i], samples)
		}
	}
}

func TestNormalizePCM16MixesStereo(t *testing.T) {
	in := encodePCM16([]int16{100, 300, 500, 700})

	got := normalizePCM16(in, openAIInputSampleRate, 2)
	samples := decodePCM16(got)
	want := []int16{200, 600}
	if len(samples) != len(want) {
		t.Fatalf("len = %d, want %d (%#v)", len(samples), len(want), samples)
	}
	for i := range want {
		if samples[i] != want[i] {
			t.Fatalf("sample[%d] = %d, want %d (%#v)", i, samples[i], want[i], samples)
		}
	}
}
