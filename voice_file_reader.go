package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

type FileVoiceReader struct {
	ctx    context.Context
	cancel context.CancelFunc
	out    chan<- string
	path   string
	once   sync.Once
}

func NewFileVoiceReader(ctx context.Context, out chan<- string, path string) *FileVoiceReader {
	runCtx, cancel := context.WithCancel(ctx)
	return &FileVoiceReader{ctx: runCtx, cancel: cancel, out: out, path: path}
}

func (f *FileVoiceReader) Run() {
	f.once.Do(func() {
		go f.loop()
	})
}

func (f *FileVoiceReader) loop() {
	file, err := os.Open(f.path)
	if err != nil {
		log.Fatalf("failed to open INPUT_VOICE: %v", err)
	}
	defer file.Close()

	info, err := parseWAV(file)
	if err != nil {
		log.Fatalf("invalid wav file: %v", err)
	}
	if info.Channels != channels || info.SampleRate != sampleRate || info.BitsPerSample != 16 {
		log.Fatalf("wav must be %d Hz, %d-bit, %d channel", sampleRate, 16, channels)
	}

	chunkSamples := sampleRate * chunkMillis / 1000
	pcmBytes := make([]byte, chunkSamples*2)
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(pcmBytes)))
	frameDuration := time.Duration(float64(chunkSamples) / float64(sampleRate) * float64(time.Second))
	due := time.Now()

	for {
		select {
		case <-f.ctx.Done():
			return
		default:
		}

		if _, err := io.ReadFull(file, pcmBytes); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				log.Println("input voice file finished streaming")
				f.flushSilence(frameDuration, encoded, len(pcmBytes))
				return
			}
			log.Fatalf("failed to read wav data: %v", err)
		}

		base64.StdEncoding.Encode(encoded, pcmBytes)
		select {
		case <-f.ctx.Done():
			return
		case f.out <- string(encoded):
		}

		due = due.Add(frameDuration)
		time.Sleep(time.Until(due))
	}
}

func (f *FileVoiceReader) Close() {
	f.cancel()
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
			var audioFormat uint16
			var numChannels uint16
			var sampleRate uint32
			var byteRate uint32
			var blockAlign uint16
			var bitsPerSample uint16
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

func (f *FileVoiceReader) flushSilence(frameDuration time.Duration, encoded []byte, pcmLen int) {
	zero := make([]byte, pcmLen)
	base64.StdEncoding.Encode(encoded, zero)
	for i := 0; i < 5; i++ {
		select {
		case <-f.ctx.Done():
			return
		case f.out <- string(encoded):
		}
		time.Sleep(frameDuration)
	}
}
