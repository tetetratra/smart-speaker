package filereader

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

const (
	fileSampleRate  = 16000
	fileChannels    = 1
	fileChunkMillis = 300
	flushSilence    = 5
)

// WAV ファイルを擬似リアルタイムで再生する
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

// WAV ファイルの形式を確認しつつバッファを初期化する
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

// 次のチャンクを読み込みリアルタイム間隔で返す
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

// 開いているファイルを閉じる
func (r *Reader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return r.file.Close()
}

type fileReader struct {
	reader          *Reader
	upstream        chan types.Event
	downstream      chan types.Event
	ctx             context.Context
	cancel          context.CancelFunc
	once            sync.Once
	closerWaitGroup sync.WaitGroup
}

// NewStage wires the file reader into the graph.Stage contract.
func NewStage(path string) (*graph.Stage, error) {
	reader, err := New(path)
	if err != nil {
		return nil, err
	}
	s := &fileReader{
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

func (s *fileReader) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	s.ctx = ctx
	s.cancel = cancel
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

func (s *fileReader) drainUpstream() {
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

func (s *fileReader) produce() {
	defer close(s.downstream)
	for {
		chunk, err := s.reader.Read(s.ctx)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("filereader stage read error: %v", err)
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

func (s *fileReader) close() error {
	var err error
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.closerWaitGroup.Wait()
		close(s.upstream)
		err = s.reader.Close()
		log.Println("filereader: stage closed")
	})
	return err
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
