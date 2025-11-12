package app

import (
	"context"
	"fmt"
	"io"

	"smart-speaker/internal/components/filereader"
	"smart-speaker/internal/components/micreader"
	"smart-speaker/internal/components/printer"
	"smart-speaker/internal/components/realtimeapi"
	"smart-speaker/internal/interfaces"
	"smart-speaker/internal/tools"
	types "smart-speaker/internal/types"
)

// Dependencies wires up all runtime components.
type Dependencies struct {
	VoiceReader    interfaces.Reader[types.AudioChunk]
	AudioProcessor interfaces.Processor[types.AudioChunk]
	ResponseReader interfaces.Reader[types.OutputLine]
	OutputWriter   interfaces.Processor[types.OutputLine]

	closers []io.Closer
}

// Close releases resources in reverse order.
func (d *Dependencies) Close() error {
	var firstErr error
	for i := len(d.closers) - 1; i >= 0; i-- {
		if err := d.closers[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Build assembles dependencies based on the config.
func Build(ctx context.Context, cfg Config) (*Dependencies, error) {
	deps := &Dependencies{}

	voice, closeVoice, err := buildVoiceReader(cfg)
	if err != nil {
		return nil, err
	}
	deps.VoiceReader = voice
	deps.closers = append(deps.closers, closeVoice)

	executor := tools.NewExecutor()

	realtimeCfg := realtimeapi.Config{
		APIKey:             cfg.APIKey,
		Model:              cfg.Model,
		TranscriptionModel: cfg.TranscriptionModel,
		Instructions:       cfg.SystemPrompt,
	}

	rtClient, err := realtimeapi.NewClient(ctx, realtimeCfg, executor)
	if err != nil {
		return nil, fmt.Errorf("realtime api: %w", err)
	}
	deps.AudioProcessor = rtClient
	deps.ResponseReader = rtClient
	deps.closers = append(deps.closers, rtClient)

	printerComp := printer.New()
	deps.OutputWriter = printerComp
	deps.closers = append(deps.closers, printerComp)

	return deps, nil
}

func buildVoiceReader(cfg Config) (interfaces.Reader[types.AudioChunk], io.Closer, error) {
	if cfg.InputVoicePath != "" {
		reader, err := filereader.New(cfg.InputVoicePath)
		if err != nil {
			return nil, nil, err
		}
		return reader, reader, nil
	}
	reader, err := micreader.New()
	if err != nil {
		return nil, nil, err
	}
	return reader, reader, nil
}
