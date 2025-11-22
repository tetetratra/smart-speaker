package micreader

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/gordonklaus/portaudio"

	"smart-speaker/internal/portaudioext"
	types "smart-speaker/internal/types"
)

const (
	sampleRate  = 16000
	channels    = 1
	chunkMillis = 300
)

// MicSource は PortAudio からマイク入力を取得する。
type MicSource struct {
	stream     *portaudio.Stream
	audioChunk []int16
	pcmBytes   []byte
	encoded    []byte
	closeOnce  sync.Once
	paAcquired bool
}

// NewMicSource は PortAudio を初期化し、デフォルト入力ストリームを開く。
func NewMicSource() (*MicSource, error) {
	if err := portaudioext.Acquire(); err != nil {
		return nil, fmt.Errorf("portaudio initialize failed: %w", err)
	}

	chunkSamples := sampleRate * chunkMillis / 1000
	audioChunk := make([]int16, chunkSamples)
	pcmBytes := make([]byte, len(audioChunk)*2)
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(pcmBytes)))

	stream, err := portaudio.OpenDefaultStream(channels, 0, float64(sampleRate), len(audioChunk), audioChunk)
	if err != nil {
		portaudioext.Release()
		return nil, fmt.Errorf("open audio stream: %w", err)
	}
	if err := stream.Start(); err != nil {
		stream.Close()
		portaudioext.Release()
		return nil, fmt.Errorf("start audio stream: %w", err)
	}

	return &MicSource{
		stream:     stream,
		audioChunk: audioChunk,
		pcmBytes:   pcmBytes,
		encoded:    encoded,
		paAcquired: true,
	}, nil
}

// Read は次の PCM チャンクを取得し Base64 で返す。
func (s *MicSource) Read(ctx context.Context) (types.AudioChunk, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := s.stream.Read(); err != nil {
		return "", fmt.Errorf("audio read failed: %w", err)
	}

	for i, sample := range s.audioChunk {
		binary.LittleEndian.PutUint16(s.pcmBytes[i*2:], uint16(sample))
	}
	base64.StdEncoding.Encode(s.encoded, s.pcmBytes)

	return types.AudioChunk(string(s.encoded)), nil
}

// Close は PortAudio のストリームを停止し、資源を解放する。
func (s *MicSource) Close() error {
	var err error
	s.closeOnce.Do(func() {
		if s.stream != nil {
			if stopErr := s.stream.Stop(); stopErr != nil && err == nil {
				err = stopErr
			}
			if closeErr := s.stream.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}
		if s.paAcquired {
			if relErr := portaudioext.Release(); relErr != nil && err == nil {
				err = relErr
			}
			s.paAcquired = false
		}
	})
	return err
}
