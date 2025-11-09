package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"log"
	"sync"

	"github.com/gordonklaus/portaudio"
)

const (
	sampleRate  = 16000
	channels    = 1
	chunkMillis = 300
)

type MicVoiceListener struct {
	ctx    context.Context
	cancel context.CancelFunc
	out    chan<- string
	once   sync.Once
}

func NewMicVoiceListener(ctx context.Context, out chan<- string) *MicVoiceListener {
	runCtx, cancel := context.WithCancel(ctx)
	return &MicVoiceListener{ctx: runCtx, cancel: cancel, out: out}
}

func (v *MicVoiceListener) Run() {
	v.once.Do(func() {
		go v.loop()
	})
}

func (v *MicVoiceListener) loop() {
	if err := portaudio.Initialize(); err != nil {
		log.Fatalf("portaudio initialize failed: %v", err)
	}
	defer portaudio.Terminate()

	chunkSamples := sampleRate * chunkMillis / 1000
	audioChunk := make([]int16, chunkSamples)
	pcmBytes := make([]byte, len(audioChunk)*2)
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(pcmBytes)))

	stream, err := portaudio.OpenDefaultStream(channels, 0, float64(sampleRate), len(audioChunk), audioChunk)
	if err != nil {
		log.Fatalf("failed to open audio stream: %v", err)
	}
	defer stream.Close()

	if err := stream.Start(); err != nil {
		log.Fatalf("failed to start audio stream: %v", err)
	}
	defer stream.Stop()

	for {
		select {
		case <-v.ctx.Done():
			return
		default:
		}

		if err := stream.Read(); err != nil {
			log.Fatalf("audio read failed: %v", err)
		}

		for i, sample := range audioChunk {
			binary.LittleEndian.PutUint16(pcmBytes[i*2:], uint16(sample))
		}
		base64.StdEncoding.Encode(encoded, pcmBytes)

		select {
		case <-v.ctx.Done():
			return
		case v.out <- string(encoded):
		}
	}
}

func (v *MicVoiceListener) Close() {
	v.cancel()
}
