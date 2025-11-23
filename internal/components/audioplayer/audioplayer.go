package audioplayer

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"

	"smart-speaker/internal/graph"
	"smart-speaker/internal/tools/portaudioext"
	types "smart-speaker/internal/types"
)

const (
	inputSampleRate  = 24000 // Realtime API のオーディオ出力は 24kHz PCM16: https://community.openai.com/t/low-and-slow-audio-from-realtime-api-how-to-properly-audio-format/1011061
	outputSampleRate = 48000
	channels         = 1
	chunkQueue       = 1024
	// PortAudio のコールバックはサンプルを要求し続けるため、即ゼロ埋めせず
	// 少しだけチャンク到着を待ってから判断する
	chunkWaitTimeout = 35 * time.Millisecond
)

type player struct {
	upstream        chan types.Event
	ctx             context.Context
	cancel          context.CancelFunc
	stream          *portaudio.Stream
	once            sync.Once
	closerWaitGroup sync.WaitGroup
	paOwned         bool

	mu      sync.Mutex
	pending []int16
	chunks  chan []int16
}

type chunkResult int

const (
	chunkResultOK chunkResult = iota
	chunkResultTimeout
	chunkResultClosed
)

func NewStage() (*graph.Stage, error) {
	if err := portaudioext.Acquire(); err != nil {
		return nil, err
	}
	p := &player{
		upstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		chunks:   make(chan []int16, chunkQueue),
		paOwned:  true,
	}
	stream, err := portaudio.OpenDefaultStream(0, channels, float64(outputSampleRate), 0, func(out []int16) {
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
		CloseFn:  p.close,
	}, nil
}

func (p *player) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	p.ctx = ctx
	p.cancel = cancel
	log.Println("🔊 音声応答を再生します。CTRL+Cで終了します。")
	p.closerWaitGroup.Add(1)
	go func() {
		defer p.closerWaitGroup.Done()
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
	// macOS など多くの出力デバイスは 48kHz が既定なので、24kHz のまま渡すと
	// PortAudio の内部リサンプラ任せになり歪みが発生しやすい。明示的にソフト側で
	// 48kHz へ変換してから再生キューへ渡すことでノイズを抑える。
	upsampled := resample24kTo48k(buf)
	if len(upsampled) == 0 {
		return nil
	}
	select {
	case p.chunks <- upsampled:
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
			chunk, result := p.waitForChunk()
			switch result {
			case chunkResultOK:
				p.pending = chunk
				continue
			case chunkResultClosed, chunkResultTimeout:
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

func (p *player) waitForChunk() ([]int16, chunkResult) {
	if p.ctx == nil {
		select {
		case chunk, ok := <-p.chunks:
			if !ok {
				return nil, chunkResultClosed
			}
			return chunk, chunkResultOK
		default:
			return nil, chunkResultTimeout
		}
	}
	t := time.NewTimer(chunkWaitTimeout)
	defer t.Stop()
	select {
	case chunk, ok := <-p.chunks:
		if !ok {
			return nil, chunkResultClosed
		}
		return chunk, chunkResultOK
	case <-p.ctx.Done():
		return nil, chunkResultClosed
	case <-t.C:
		return nil, chunkResultTimeout
	}
}

func resample24kTo48k(samples []int16) []int16 {
	if len(samples) == 0 {
		return nil
	}
	out := make([]int16, len(samples)*2)
	last := len(samples) - 1
	for i := 0; i < last; i++ {
		s := int32(samples[i])
		next := int32(samples[i+1])
		out[2*i] = samples[i]
		out[2*i+1] = int16((s + next) / 2)
	}
	out[2*last] = samples[last]
	out[2*last+1] = samples[last]
	return out
}

func (p *player) close() error {
	var err error
	p.once.Do(func() {
		if p.cancel != nil {
			p.cancel()
		}
		close(p.upstream)
		p.closerWaitGroup.Wait()
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
