package playbackspeed

import (
	"encoding/base64"
	"encoding/binary"
	"math"
	"testing"
)

func pcmFromSamples(samples []int16) string {
	raw := make([]byte, len(samples)*pcmBytesPerSample)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(raw[i*pcmBytesPerSample:], uint16(sample))
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func samplesFromPCM(base64PCM string) ([]int16, error) {
	raw, err := base64.StdEncoding.DecodeString(base64PCM)
	if err != nil {
		return nil, err
	}
	out := make([]int16, len(raw)/pcmBytesPerSample)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(raw[i*pcmBytesPerSample:]))
	}
	return out, nil
}

func TestCompressPCMReturnsOriginalAtOneX(t *testing.T) {
	in := pcmFromSamples([]int16{100, 200, 300, 400})
	out, err := compressPCM(in, 1)
	if err != nil {
		t.Fatalf("compressPCM: %v", err)
	}
	if out != in {
		t.Fatalf("output changed at 1x")
	}
}

func TestCompressPCMShortensAtTwoX(t *testing.T) {
	in := pcmFromSamples([]int16{0, 1000, 2000, 3000, 4000, 5000})
	out, err := compressPCM(in, 2)
	if err != nil {
		t.Fatalf("compressPCM: %v", err)
	}
	samples, err := samplesFromPCM(out)
	if err != nil {
		t.Fatalf("samplesFromPCM: %v", err)
	}
	if len(samples) != 3 {
		t.Fatalf("len(samples) = %d, want 3", len(samples))
	}
	if math.Abs(float64(samples[1])-2000) > 1 {
		t.Fatalf("sample[1] = %d, want ~2000", samples[1])
	}
}

func TestCompressPCMRejectsMisalignedInput(t *testing.T) {
	_, err := compressPCM(base64.StdEncoding.EncodeToString([]byte{0, 1, 2}), 2)
	if err == nil {
		t.Fatal("expected error for misaligned pcm")
	}
}
