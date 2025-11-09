package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type (
	wsMessage map[string]any

	OutputLine struct {
		Role string
		Text string
	}

	config struct {
		APIKey             string
		Model              string
		TranscriptionModel string
	}

	VoiceSource interface {
		Run()
		Close()
	}
)

func main() {
	log.SetFlags(0)

	cfg := loadConfig()
	ctx := context.Background()

	userVoiceStream := make(chan string, 16)
	responseTextStream := make(chan OutputLine, 16)

	voice := newVoiceSource(ctx, userVoiceStream)
	api, err := NewRealtimeAPI(ctx, cfg, userVoiceStream, responseTextStream)
	if err != nil {
		log.Fatalf("failed to connect realtime api: %v", err)
	}
	printer := NewResponsePrinter(ctx, responseTextStream)

	voice.Run()
	api.Run()
	printer.Run()

	defer voice.Close()
	defer api.Close()
	defer printer.Close()

	fmt.Println("🎙 音声ストリーミングを開始しました。Ctrl+C で終了します。")
	select {}
}

func loadConfig() config {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY is not set")
	}

	model := os.Getenv("OPENAI_REALTIME_MODEL")
	if model == "" {
		model = "gpt-realtime"
	}

	transcription := os.Getenv("OPENAI_TRANSCRIPTION_MODEL")
	if transcription == "" {
		transcription = "gpt-4o-transcribe"
	}

	return config{
		APIKey:             apiKey,
		Model:              model,
		TranscriptionModel: transcription,
	}
}

func newVoiceSource(ctx context.Context, out chan<- string) VoiceSource {
	path := strings.TrimSpace(os.Getenv("INPUT_VOICE"))
	if path == "" {
		return NewMicVoiceListener(ctx, out)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		log.Fatalf("failed to resolve INPUT_VOICE path: %v", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		log.Fatalf("INPUT_VOICE file not found: %v", err)
	}
	if info.IsDir() {
		log.Fatal("INPUT_VOICE must point to a file")
	}
	log.Printf("Using INPUT_VOICE file: %s", abs)
	return NewFileVoiceReader(ctx, out, abs)
}
