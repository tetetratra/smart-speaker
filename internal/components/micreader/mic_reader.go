package micreader

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/gordonklaus/portaudio"

	"smart-speaker/internal/graph"
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
var _ graph.Stage = (*Stage)(nil)

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

// Stage exposes microphone reader as graph.Stage.
type Stage struct {
	reader     *Reader
	upstream   chan interface{}
	downstream chan interface{}
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once
}

func NewStage() (*Stage, error) {
	reader, err := New()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Stage{
		reader:     reader,
		upstream:   make(chan interface{}),
		downstream: make(chan interface{}),
		ctx:        ctx,
		cancel:     cancel,
	}
	go s.drainUpstream()
	go s.produce()
	return s, nil
}

func (s *Stage) drainUpstream() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case _, ok := <-s.upstream:
			if !ok {
				return
			}
		}
	}
}

func (s *Stage) produce() {
	defer close(s.downstream)
	for {
		chunk, err := s.reader.Read(s.ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
				return
			}
			log.Printf("micreader stage read error: %v", err)
			return
		}
		evt := types.Event{Kind: types.EventAudioChunk, Payload: chunk}
		select {
		case <-s.ctx.Done():
			return
		case s.downstream <- evt:
		}
	}
}

func (s *Stage) Upstream() chan<- interface{} { return s.upstream }

func (s *Stage) Downstream() <-chan interface{} { return s.downstream }

func (s *Stage) Close() error {
	var err error
	s.once.Do(func() {
		s.cancel()
		close(s.upstream)
		err = s.reader.Close()
	})
	return err
}
