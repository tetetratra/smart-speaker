package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os/signal"
	"sync"
	"syscall"

	"smart-speaker/internal/app"
	"smart-speaker/internal/interfaces"
	types "smart-speaker/internal/types"
)

func main() {
	log.SetFlags(0)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := app.LoadConfig("system_prompt.txt")

	deps, err := app.Build(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to build dependencies: %v", err)
	}
	defer deps.Close()

	audioCh := make(chan types.AudioChunk)
	outputCh := make(chan types.OutputLine)

	var wg sync.WaitGroup

	startReader(ctx, &wg, "voice", deps.VoiceReader, audioCh, cancel)
	startProcessor(ctx, &wg, "audio", deps.AudioProcessor, audioCh, cancel)
	startReader(ctx, &wg, "realtime", deps.ResponseReader, outputCh, cancel)
	startProcessor(ctx, &wg, "printer", deps.OutputWriter, outputCh, cancel)

	fmt.Println("🎙 音声ストリーミングを開始しました。Ctrl+C で終了します。")
	wg.Wait()
}

func startReader[T any](ctx context.Context, wg *sync.WaitGroup, name string, reader interfaces.Reader[T], out chan<- T, cancel context.CancelFunc) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(out)
		for {
			data, err := reader.Read(ctx)
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				if ctx.Err() == nil {
					log.Printf("%s reader error: %v", name, err)
				}
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			case out <- data:
			}
		}
	}()
}

func startProcessor[T any](ctx context.Context, wg *sync.WaitGroup, name string, processor interfaces.Processor[T], in <-chan T, cancel context.CancelFunc) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		for data := range in {
			if err := processor.Process(ctx, data); err != nil {
				if ctx.Err() == nil {
					log.Printf("%s processor error: %v", name, err)
				}
				cancel()
				return
			}
		}
	}()
}
