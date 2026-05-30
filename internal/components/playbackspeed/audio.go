package playbackspeed

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
)

const (
	pcmSampleRate     = 24000
	pcmBytesPerSample = 2
	pcmChannels       = 1
)

func compressPCM(base64PCM string, speed float64) (string, error) {
	if speed <= 0 || math.Abs(speed-1) < 1e-9 {
		return base64PCM, nil
	}
	raw, err := base64.StdEncoding.DecodeString(base64PCM)
	if err != nil {
		return "", fmt.Errorf("decode pcm: %w", err)
	}
	if len(raw) < pcmBytesPerSample {
		return base64PCM, nil
	}
	if len(raw)%pcmBytesPerSample != 0 {
		return "", fmt.Errorf("pcm byte length %d is not aligned to sample size", len(raw))
	}

	inputSamples := len(raw) / pcmBytesPerSample
	outputSamples := int(math.Floor(float64(inputSamples) / speed))
	if outputSamples <= 0 {
		return base64.StdEncoding.EncodeToString(nil), nil
	}

	out := make([]byte, outputSamples*pcmBytesPerSample)
	for i := 0; i < outputSamples; i++ {
		pos := float64(i) * speed
		sample := interpolateSample(raw, pos)
		binary.LittleEndian.PutUint16(out[i*pcmBytesPerSample:], uint16(sample))
	}
	return base64.StdEncoding.EncodeToString(out), nil
}

func interpolateSample(pcm []byte, position float64) int16 {
	sampleCount := len(pcm) / pcmBytesPerSample
	if sampleCount == 0 {
		return 0
	}
	if position <= 0 {
		return int16(binary.LittleEndian.Uint16(pcm))
	}
	maxIndex := float64(sampleCount - 1)
	if position >= maxIndex {
		offset := (sampleCount - 1) * pcmBytesPerSample
		return int16(binary.LittleEndian.Uint16(pcm[offset:]))
	}

	lower := int(math.Floor(position))
	upper := lower + 1
	frac := position - float64(lower)

	lowerSample := int16(binary.LittleEndian.Uint16(pcm[lower*pcmBytesPerSample:]))
	upperSample := int16(binary.LittleEndian.Uint16(pcm[upper*pcmBytesPerSample:]))
	return int16(math.Round(float64(lowerSample)*(1-frac) + float64(upperSample)*frac))
}
