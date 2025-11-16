package audioplayer

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/gordonklaus/portaudio"

	"smart-speaker/internal/graph"
	"smart-speaker/internal/portaudioext"
	types "smart-speaker/internal/types"
)

const (
	sampleRate = 16000
	channels   = 1
	chunkQueue = 32
)

type player struct {
	upstream chan types.Event
	ctx      context.Context
	cancel   context.CancelFunc
	stream   *portaudio.Stream
	once     sync.Once
	lineWG   sync.WaitGroup
	paOwned  bool

	mu      sync.Mutex
	pending []int16
	chunks  chan []int16
}

func NewStage() (*graph.Stage, error) {
	if err := portaudioext.Acquire(); err != nil {
		return nil, err
	}
	p := &player{
		upstream: make(chan types.Event),
		chunks:   make(chan []int16, chunkQueue),
		paOwned:  true,
	}
	stream, err := portaudio.OpenDefaultStream(0, channels, float64(sampleRate), 0, func(out []int16) {
		p.fill(out)
	})
	if err != nil {
		portaudioext.Release()
		return nil, fmt.Errorf("open audio output: %w", err)
	}
	if err := stream.Start(); err != nil {
		stream.Close()
		portaudioext.Release()
		return nil, fmt.Errorf("start audio output: %w", err)
	}
	p.stream = stream
	return &graph.Stage{
		Upstream: p.upstream,
		Run:      p.run,
		CloseFn:  p.Close,
	}, nil
}

func (p *player) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	p.ctx = ctx
	p.cancel = cancel
	log.Println("🔊 音声応答を再生します。CTRL+Cで終了します。")
	p.lineWG.Add(1)
	go func() {
		defer p.lineWG.Done()
		p.consume()
	}()
}

func (p *player) consume() {
	for {
		select {
		case <-p.ctx.Done():
			return
		case evt, ok := <-p.upstream:
			if !ok {
				return
			}
			if evt.Kind != types.EventRealtimeAudio {
				continue
			}
			audio, ok := evt.Payload.(types.OutputAudio)
			if !ok {
				continue
			}
			if err := p.enqueue(audio); err != nil {
				log.Printf("audioplayer: enqueue error: %v", err)
			}
		}
	}
}

func (p *player) enqueue(chunk types.OutputAudio) error {
	data, err := base64.StdEncoding.DecodeString(chunk.Audio)
	if err != nil {
		return fmt.Errorf("decode chunk: %w", err)
	}
	if len(data)%2 != 0 {
		data = data[:len(data)-1]
	}
	samples := len(data) / 2
	if samples == 0 {
		return nil
	}
	buf := make([]int16, samples)
	for i := 0; i < samples; i++ {
		buf[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	select {
	case p.chunks <- buf:
	default:
		return errors.New("audio buffer overflow")
	}
	return nil
}

func (p *player) fill(out []int16) {
	p.mu.Lock()
	defer p.mu.Unlock()
	copied := 0
	for copied < len(out) {
		if len(p.pending) == 0 {
			select {
			case chunk, ok := <-p.chunks:
				if !ok {
					for i := copied; i < len(out); i++ {
						out[i] = 0
					}
					return
				}
				p.pending = chunk
			default:
				for i := copied; i < len(out); i++ {
					out[i] = 0
				}
				return
			}
		}
		n := copy(out[copied:], p.pending)
		copied += n
		p.pending = p.pending[n:]
	}
}

func (p *player) Close() error {
	var err error
	p.once.Do(func() {
		if p.cancel != nil {
			p.cancel()
		}
		close(p.upstream)
		p.lineWG.Wait()
		close(p.chunks)
		if p.stream != nil {
			if stopErr := p.stream.Stop(); stopErr != nil && err == nil {
				err = stopErr
			}
			if closeErr := p.stream.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}
		if p.paOwned {
			if relErr := portaudioext.Release(); relErr != nil && err == nil {
				err = relErr
			}
			p.paOwned = false
		}
		log.Println("audioplayer: stage closed")
	})
	return err
}
