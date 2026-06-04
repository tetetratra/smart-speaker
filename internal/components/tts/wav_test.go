package tts

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestExtractPCMFromWAV(t *testing.T) {
	pcm := []byte{1, 2, 3, 4}
	got, err := extractPCMFromWAV(testWAV(t, wavOptions{PCM: pcm}))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(pcm) {
		t.Fatalf("pcm = %v, want %v", got, pcm)
	}
}

func TestExtractPCMFromWAVRejectsUnsupportedFormats(t *testing.T) {
	tests := []struct {
		name string
		opts wavOptions
		want string
	}{
		{name: "non pcm", opts: wavOptions{AudioFormat: 3}, want: "audio format"},
		{name: "wrong sample rate", opts: wavOptions{SampleRate: 48000}, want: "sample rate"},
		{name: "wrong bits", opts: wavOptions{BitsPerSample: 24}, want: "bits per sample"},
		{name: "wrong channels", opts: wavOptions{Channels: 2}, want: "channels"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractPCMFromWAV(testWAV(t, tt.opts))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestExtractPCMFromWAVRejectsMissingDataChunk(t *testing.T) {
	wav := testWAV(t, wavOptions{OmitData: true})
	_, err := extractPCMFromWAV(wav)
	if err == nil || !strings.Contains(err.Error(), "data chunk") {
		t.Fatalf("error = %v, want data chunk error", err)
	}
}

type wavOptions struct {
	PCM           []byte
	AudioFormat   uint16
	Channels      uint16
	SampleRate    uint32
	BitsPerSample uint16
	OmitData      bool
}

func testWAV(t *testing.T, opts wavOptions) []byte {
	t.Helper()
	if opts.PCM == nil {
		opts.PCM = []byte{1, 2}
	}
	if opts.AudioFormat == 0 {
		opts.AudioFormat = wavFormatPCM
	}
	if opts.Channels == 0 {
		opts.Channels = ttsPCMOutputChannels
	}
	if opts.SampleRate == 0 {
		opts.SampleRate = ttsPCMOutputSampleRate
	}
	if opts.BitsPerSample == 0 {
		opts.BitsPerSample = ttsPCMOutputBytesPerSample * 8
	}

	blockAlign := opts.Channels * opts.BitsPerSample / 8
	byteRate := opts.SampleRate * uint32(blockAlign)
	fmtChunk := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtChunk[0:2], opts.AudioFormat)
	binary.LittleEndian.PutUint16(fmtChunk[2:4], opts.Channels)
	binary.LittleEndian.PutUint32(fmtChunk[4:8], opts.SampleRate)
	binary.LittleEndian.PutUint32(fmtChunk[8:12], byteRate)
	binary.LittleEndian.PutUint16(fmtChunk[12:14], blockAlign)
	binary.LittleEndian.PutUint16(fmtChunk[14:16], opts.BitsPerSample)

	chunks := make([]byte, 0)
	chunks = appendChunk(chunks, "fmt ", fmtChunk)
	if !opts.OmitData {
		chunks = appendChunk(chunks, "data", opts.PCM)
	}
	out := make([]byte, 12, 12+len(chunks))
	copy(out[0:4], "RIFF")
	binary.LittleEndian.PutUint32(out[4:8], uint32(4+len(chunks)))
	copy(out[8:12], "WAVE")
	out = append(out, chunks...)
	return out
}

func appendChunk(dst []byte, id string, data []byte) []byte {
	dst = append(dst, []byte(id)...)
	size := make([]byte, 4)
	binary.LittleEndian.PutUint32(size, uint32(len(data)))
	dst = append(dst, size...)
	dst = append(dst, data...)
	if len(data)%2 == 1 {
		dst = append(dst, 0)
	}
	return dst
}
