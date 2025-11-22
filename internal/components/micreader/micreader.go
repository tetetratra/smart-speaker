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
	"smart-speaker/internal/portaudioext"
	types "smart-speaker/internal/types"
)

const (
	sampleRate  = 16000
	channels    = 1
	chunkMillis = 300
)

// マイク入力を同期的に取得する
type Reader struct {
	stream     *portaudio.Stream
	audioChunk []int16
	pcmBytes   []byte
	encoded    []byte
	closeOnce  sync.Once
	paAcquired bool
}

// PortAudio を初期化してデフォルト入力ストリームを開く
func New() (*Reader, error) {
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

	return &Reader{
		stream:     stream,
		audioChunk: audioChunk,
		pcmBytes:   pcmBytes,
		encoded:    encoded,
		paAcquired: true,
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
		if r.paAcquired {
			if relErr := portaudioext.Release(); relErr != nil && err == nil {
				err = relErr
			}
			r.paAcquired = false
		}
	})
	return err
}

type micReader struct {
	reader          *Reader
	upstream        chan types.Event
	downstream      chan types.Event
	ctx             context.Context
	cancel          context.CancelFunc
	once            sync.Once
	closerWaitGroup sync.WaitGroup
}

// NewStage exposes microphone reader as graph.Stage.
func NewStage() (*graph.Stage, error) {
	reader, err := New()
	if err != nil {
		return nil, err
	}
	s := &micReader{
		reader:     reader,
		upstream:   make(chan types.Event),
		downstream: make(chan types.Event),
	}
	return &graph.Stage{
		Upstream:   s.upstream,
		Downstream: s.downstream,
		Run:        s.run,
		CloseFn:    s.close,
	}, nil
}

func (s *micReader) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.ctx = ctx
	s.cancel = cancel
	log.Println("🎙 マイク入力を待機しています。CTRL+Cで終了します。")
	s.closerWaitGroup.Add(2)
	go func() {
		defer s.closerWaitGroup.Done()
		s.drainUpstream()
	}()
	go func() {
		defer s.closerWaitGroup.Done()
		s.produce()
	}()
}

func (s *micReader) drainUpstream() {
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

func (s *micReader) produce() {
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

func (s *micReader) close() error {
	var err error
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.closerWaitGroup.Wait()
		close(s.upstream)
		err = s.reader.Close()
		log.Println("micreader: stage closed")
	})
	return err
}
