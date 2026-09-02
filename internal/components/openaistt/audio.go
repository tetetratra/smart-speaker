package openaistt

import "encoding/binary"

const (
	openAIInputSampleRate = 24000
	openAIInputChannels   = 1
	openAIChunkBytes      = 25600
)

func normalizePCM16(pcm []byte, sampleRate int, channels int) []byte {
	if len(pcm) < 2 {
		return nil
	}
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if channels <= 0 {
		channels = 1
	}
	samples := decodePCM16(pcm)
	mono := toMono(samples, channels)
	if sampleRate == openAIInputSampleRate {
		return encodePCM16(mono)
	}
	return encodePCM16(resampleLinear(mono, sampleRate, openAIInputSampleRate))
}

func decodePCM16(pcm []byte) []int16 {
	n := len(pcm) / 2
	out := make([]int16, n)
	for i := 0; i < n; i++ {
		out[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}
	return out
}

func encodePCM16(samples []int16) []byte {
	if len(samples) == 0 {
		return nil
	}
	out := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(sample))
	}
	return out
}

func toMono(samples []int16, channels int) []int16 {
	if channels <= 1 {
		return samples
	}
	frames := len(samples) / channels
	out := make([]int16, frames)
	for frame := 0; frame < frames; frame++ {
		sum := 0
		for ch := 0; ch < channels; ch++ {
			sum += int(samples[frame*channels+ch])
		}
		out[frame] = int16(sum / channels)
	}
	return out
}

func resampleLinear(samples []int16, fromRate int, toRate int) []int16 {
	if len(samples) == 0 || fromRate <= 0 || toRate <= 0 {
		return nil
	}
	if fromRate == toRate {
		return samples
	}
	outLen := int((int64(len(samples))*int64(toRate) + int64(fromRate) - 1) / int64(fromRate))
	if outLen <= 0 {
		return nil
	}
	out := make([]int16, outLen)
	for i := range out {
		pos := float64(i) * float64(fromRate) / float64(toRate)
		left := int(pos)
		if left >= len(samples)-1 {
			out[i] = samples[len(samples)-1]
			continue
		}
		frac := pos - float64(left)
		a := float64(samples[left])
		b := float64(samples[left+1])
		out[i] = int16(a + (b-a)*frac)
	}
	return out
}
