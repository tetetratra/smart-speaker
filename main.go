package main

import (
	"context"
	"fmt"
	"log"
	"os"
)

type (
	wsMessage  map[string]any
	OutputLine struct {
		Role string
		Text string
	}

	config struct {
		APIKey             string
		Model              string
		TranscriptionModel string
	}
)

func main() {
	log.SetFlags(0)

	cfg := loadConfig()

	ctx := context.Background()

	userVoiceStream := make(chan string, 16)
	responseTextStream := make(chan OutputLine, 16)

	api, err := NewRealtimeAPI(ctx, cfg, userVoiceStream, responseTextStream)
	if err != nil {
		log.Fatalf("failed to connect realtime api: %v", err)
	}
	voice := NewVoiceListener(ctx, userVoiceStream)
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

	// MEMO: この文字起こしがAIに渡るわけではない
	transcription := os.Getenv("OPENAI_TRANSCRIPTION_MODEL")
	if transcription == "" {
		// transcription = "gpt-4o-transcribe"
		transcription = "whisper-1"
	}

	return config{
		APIKey:             apiKey,
		Model:              model,
		TranscriptionModel: transcription,
	}
}
