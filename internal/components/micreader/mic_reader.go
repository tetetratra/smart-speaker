package micreader

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/gordonklaus/portaudio"

	"smart-speaker/internal/interfaces"
	types "smart-speaker/internal/types"
)

const (
	sampleRate  = 16000
	channels    = 1
	chunkMillis = 300
)

// Reader がインターフェースを満たしているか確認する
var _ interfaces.Reader[types.AudioChunk] = (*Reader)(nil)

// マイク入力を同期的に取得する
type Reader struct {
	stream     *portaudio.Stream
	audioChunk []int16
	pcmBytes   []byte
	encoded    []byte
	closeOnce  sync.Once
}

// PortAudio を初期化してデフォルト入力ストリームを開く
func New() (*Reader, error) {
	if err := portaudio.Initialize(); err != nil {
		return nil, fmt.Errorf("portaudio initialize failed: %w", err)
	}

	chunkSamples := sampleRate * chunkMillis / 1000
	audioChunk := make([]int16, chunkSamples)
	pcmBytes := make([]byte, len(audioChunk)*2)
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(pcmBytes)))

	stream, err := portaudio.OpenDefaultStream(channels, 0, float64(sampleRate), len(audioChunk), audioChunk)
	if err != nil {
		portaudio.Terminate()
		return nil, fmt.Errorf("open audio stream: %w", err)
	}
	if err := stream.Start(); err != nil {
		stream.Close()
		portaudio.Terminate()
		return nil, fmt.Errorf("start audio stream: %w", err)
	}

	return &Reader{
		stream:     stream,
		audioChunk: audioChunk,
		pcmBytes:   pcmBytes,
		encoded:    encoded,
	}, nil
}

// 次の PCM チャンクを取得し Base64 で返す
func (r *Reader) Read(ctx context.Context) (types.AudioChunk, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	if err := r.stream.Read(); err != nil {
		return "", fmt.Errorf("audio read failed: %w", err)
	}

	for i, sample := range r.audioChunk {
		binary.LittleEndian.PutUint16(r.pcmBytes[i*2:], uint16(sample))
	}
	base64.StdEncoding.Encode(r.encoded, r.pcmBytes)

	return types.AudioChunk(string(r.encoded)), nil
}

// PortAudio のストリームを停止して解放する
func (r *Reader) Close() error {
	var err error
	r.closeOnce.Do(func() {
		if r.stream != nil {
			if stopErr := r.stream.Stop(); stopErr != nil && err == nil {
				err = stopErr
			}
			if closeErr := r.stream.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}
		portaudio.Terminate()
	})
	return err
}
