package filereader

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"smart-speaker/internal/interfaces"
	types "smart-speaker/internal/types"
)

const (
	fileSampleRate  = 16000
	fileChannels    = 1
	fileChunkMillis = 300
	flushSilence    = 5
)

// Ensure interface compliance.
var _ interfaces.Reader[types.AudioChunk] = (*Reader)(nil)

// Reader replays audio from a WAV file.
type Reader struct {
	file          *os.File
	pcmBytes      []byte
	encoded       []byte
	frameDuration time.Duration
	silenceLeft   int
	pendingEOF    bool
	mu            sync.Mutex
	closed        bool
}

// New validates the WAV file and prepares buffers.
func New(path string) (*Reader, error) {
	abs, err := resolvePath(path)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("open input voice: %w", err)
	}

	info, err := parseWAV(file)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("invalid wav file: %w", err)
	}
	if info.Channels != fileChannels || info.SampleRate != fileSampleRate || info.BitsPerSample != 16 {
		file.Close()
		return nil, fmt.Errorf("wav must be %d Hz, 16-bit, %d channel", fileSampleRate, fileChannels)
	}

	chunkSamples := fileSampleRate * fileChunkMillis / 1000
	pcm := make([]byte, chunkSamples*2)
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(pcm)))
	frameDuration := time.Duration(float64(chunkSamples) / float64(fileSampleRate) * float64(time.Second))

	return &Reader{
		file:          file,
		pcmBytes:      pcm,
		encoded:       encoded,
		frameDuration: frameDuration,
	}, nil
}

// Read returns the next chunk, replaying real-time pacing.
func (r *Reader) Read(ctx context.Context) (types.AudioChunk, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return "", io.EOF
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}

	if r.silenceLeft > 0 {
		return r.emitSilence(), nil
	}

	if _, err := io.ReadFull(r.file, r.pcmBytes); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			r.silenceLeft = flushSilence
			r.pendingEOF = true
			return r.emitSilence(), nil
		}
		return "", fmt.Errorf("read wav data: %w", err)
	}

	base64.StdEncoding.Encode(r.encoded, r.pcmBytes)
	time.Sleep(r.frameDuration)
	return types.AudioChunk(string(r.encoded)), nil
}

func (r *Reader) emitSilence() types.AudioChunk {
	zero := make([]byte, len(r.pcmBytes))
	base64.StdEncoding.Encode(r.encoded, zero)
	r.silenceLeft--
	if r.silenceLeft == 0 && r.pendingEOF {
		r.closed = true
	}
	time.Sleep(r.frameDuration)
	return types.AudioChunk(string(r.encoded))
}

// Close closes the underlying file.
func (r *Reader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return r.file.Close()
}

type wavMeta struct {
	Channels      int
	SampleRate    int
	BitsPerSample int
}

func parseWAV(r io.ReadSeeker) (*wavMeta, error) {
	var riff [4]byte
	if _, err := io.ReadFull(r, riff[:]); err != nil {
		return nil, err
	}
	if string(riff[:]) != "RIFF" {
		return nil, fmt.Errorf("not RIFF")
	}
	if _, err := r.Seek(4, io.SeekCurrent); err != nil {
		return nil, err
	}
	var wave [4]byte
	if _, err := io.ReadFull(r, wave[:]); err != nil {
		return nil, err
	}
	if string(wave[:]) != "WAVE" {
		return nil, fmt.Errorf("not WAVE")
	}

	meta := &wavMeta{}
	for {
		var chunkID [4]byte
		if _, err := io.ReadFull(r, chunkID[:]); err != nil {
			return nil, err
		}
		var chunkSize uint32
		if err := binary.Read(r, binary.LittleEndian, &chunkSize); err != nil {
			return nil, err
		}
		switch string(chunkID[:]) {
		case "fmt ":
			var audioFormat, numChannels, bitsPerSample uint16
			var sampleRate, byteRate uint32
			var blockAlign uint16
			if err := binary.Read(r, binary.LittleEndian, &audioFormat); err != nil {
				return nil, err
			}
			if err := binary.Read(r, binary.LittleEndian, &numChannels); err != nil {
				return nil, err
			}
			if err := binary.Read(r, binary.LittleEndian, &sampleRate); err != nil {
				return nil, err
			}
			if err := binary.Read(r, binary.LittleEndian, &byteRate); err != nil {
				return nil, err
			}
			if err := binary.Read(r, binary.LittleEndian, &blockAlign); err != nil {
				return nil, err
			}
			if err := binary.Read(r, binary.LittleEndian, &bitsPerSample); err != nil {
				return nil, err
			}
			if audioFormat != 1 {
				return nil, fmt.Errorf("unsupported wav format %d", audioFormat)
			}
			meta.Channels = int(numChannels)
			meta.SampleRate = int(sampleRate)
			meta.BitsPerSample = int(bitsPerSample)

			remaining := int64(chunkSize) - 16
			if remaining > 0 {
				if _, err := r.Seek(remaining, io.SeekCurrent); err != nil {
					return nil, err
				}
			}
		case "data":
			return meta, nil
		default:
			if _, err := r.Seek(int64(chunkSize), io.SeekCurrent); err != nil {
				return nil, err
			}
		}
	}
}

func resolvePath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("path is empty")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("INPUT_VOICE must point to a file")
	}
	return abs, nil
}
