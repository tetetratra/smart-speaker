package tts

import (
	"encoding/binary"
	"fmt"
)

const (
	wavFormatPCM = 1
)

func extractPCMFromWAV(data []byte) ([]byte, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("wav: header is too short")
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("wav: unsupported header")
	}

	var (
		seenFmt  bool
		seenData bool
		pcm      []byte
	)
	for offset := 12; offset+8 <= len(data); {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		offset += 8
		if chunkSize < 0 || offset+chunkSize > len(data) {
			return nil, fmt.Errorf("wav: invalid %s chunk size", chunkID)
		}
		chunk := data[offset : offset+chunkSize]
		switch chunkID {
		case "fmt ":
			if err := validateWAVFormat(chunk); err != nil {
				return nil, err
			}
			seenFmt = true
		case "data":
			pcm = append([]byte(nil), chunk...)
			seenData = true
		}
		offset += chunkSize
		if chunkSize%2 == 1 {
			offset++
		}
	}
	if !seenFmt {
		return nil, fmt.Errorf("wav: fmt chunk is missing")
	}
	if !seenData {
		return nil, fmt.Errorf("wav: data chunk is missing")
	}
	return pcm, nil
}

func validateWAVFormat(chunk []byte) error {
	if len(chunk) < 16 {
		return fmt.Errorf("wav: fmt chunk is too short")
	}
	audioFormat := binary.LittleEndian.Uint16(chunk[0:2])
	channels := binary.LittleEndian.Uint16(chunk[2:4])
	sampleRate := binary.LittleEndian.Uint32(chunk[4:8])
	bitsPerSample := binary.LittleEndian.Uint16(chunk[14:16])

	if audioFormat != wavFormatPCM {
		return fmt.Errorf("wav: audio format must be PCM, got %d", audioFormat)
	}
	if sampleRate != ttsPCMOutputSampleRate {
		return fmt.Errorf("wav: sample rate must be %d, got %d", ttsPCMOutputSampleRate, sampleRate)
	}
	if bitsPerSample != ttsPCMOutputBytesPerSample*8 {
		return fmt.Errorf("wav: bits per sample must be %d, got %d", ttsPCMOutputBytesPerSample*8, bitsPerSample)
	}
	if channels != ttsPCMOutputChannels {
		return fmt.Errorf("wav: channels must be %d, got %d", ttsPCMOutputChannels, channels)
	}
	return nil
}
