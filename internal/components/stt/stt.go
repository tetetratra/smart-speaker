package stt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	speech "cloud.google.com/go/speech/apiv2"
	speechpb "cloud.google.com/go/speech/apiv2/speechpb"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func (s *stage) handleSpeechAudio(audio types.RTCSpeechAudio) {
	switch audio.Type {
	case types.RTCSpeechAudioStart:
		s.startSpeechStream(int32(audio.SampleRate), int32(audio.Channels), audio.Prebuffer)
	case types.RTCSpeechAudioFrame:
		s.sendSpeechAudio(audio.PCM)
	case types.RTCSpeechAudioEnd:
		s.scheduleSpeechStop()
	}
}

func (s *stage) startSpeechStream(sampleRate int32, channels int32, prebuffer []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cancelSpeechStopLocked()
	s.stopSpeechLocked()

	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if channels <= 0 {
		channels = 1
	}

	projectID := strings.TrimSpace(s.cfg.SpeechProjectID)
	if projectID == "" {
		log.Printf("stt: GOOGLE_CLOUD_PROJECT is empty; server-side STT disabled")
		return
	}
	if s.speechClient == nil {
		opts := make([]option.ClientOption, 0, 2)
		opts = append(opts, option.WithEndpoint(fmt.Sprintf("%s-speech.googleapis.com:443", speechRegion)))
		if strings.TrimSpace(s.cfg.SpeechCredsJSON) != "" {
			opts = append(opts, option.WithCredentialsJSON([]byte(s.cfg.SpeechCredsJSON)))
		}
		client, err := speech.NewClient(s.ctx, opts...)
		if err != nil {
			log.Printf("stt: speech client create error: %v", err)
			return
		}
		s.speechClient = client
	}

	speechCtx, cancel := context.WithCancel(s.ctx)
	stream, err := s.speechClient.StreamingRecognize(speechCtx)
	if err != nil {
		cancel()
		log.Printf("stt: speech streaming create error: %v", err)
		return
	}
	req := &speechpb.StreamingRecognizeRequest{
		Recognizer: recognizerPath(projectID, s.cfg.SpeechRecognizer),
		StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
			StreamingConfig: &speechpb.StreamingRecognitionConfig{
				Config: &speechpb.RecognitionConfig{
					Model:      speechModel,
					Adaptation: buildSpeechAdaptation(s.cfg.SpeechPhrases),
					DecodingConfig: &speechpb.RecognitionConfig_ExplicitDecodingConfig{
						ExplicitDecodingConfig: &speechpb.ExplicitDecodingConfig{
							Encoding:          speechpb.ExplicitDecodingConfig_LINEAR16,
							SampleRateHertz:   sampleRate,
							AudioChannelCount: channels,
						},
					},
					LanguageCodes: []string{s.cfg.SpeechLanguage},
				},
			},
		},
	}
	if err := stream.Send(req); err != nil {
		_ = stream.CloseSend()
		cancel()
		log.Printf("stt: speech stream config send error: %v", err)
		return
	}

	s.speechStream = stream
	s.speechCancel = cancel
	go s.consumeSpeechResponses(stream)

	if len(prebuffer) > 0 {
		go s.sendSpeechAudio(prebuffer)
	}
}

func buildSpeechAdaptation(phrases []string) *speechpb.SpeechAdaptation {
	if len(phrases) == 0 {
		return nil
	}
	pbPhrases := make([]*speechpb.PhraseSet_Phrase, 0, len(phrases))
	for _, phrase := range phrases {
		trimmed := strings.TrimSpace(phrase)
		if trimmed == "" {
			continue
		}
		pbPhrases = append(pbPhrases, &speechpb.PhraseSet_Phrase{Value: trimmed})
	}
	if len(pbPhrases) == 0 {
		return nil
	}
	return &speechpb.SpeechAdaptation{
		PhraseSets: []*speechpb.SpeechAdaptation_AdaptationPhraseSet{
			{
				Value: &speechpb.SpeechAdaptation_AdaptationPhraseSet_InlinePhraseSet{
					InlinePhraseSet: &speechpb.PhraseSet{
						Phrases: pbPhrases,
						Boost:   20,
					},
				},
			},
		},
	}
}

func (s *stage) consumeSpeechResponses(stream speechpb.Speech_StreamingRecognizeClient) {
	for {
		resp, err := stream.Recv()
		if err != nil {
			if !isExpectedSpeechStreamClose(err) {
				log.Printf("stt: speech stream recv error: %v", err)
			}
			return
		}
		for _, result := range resp.Results {
			if !result.IsFinal {
				continue
			}
			if len(result.Alternatives) == 0 {
				continue
			}
			text := strings.TrimSpace(result.Alternatives[0].Transcript)
			if text == "" {
				continue
			}
			s.emit(types.Event{Kind: types.EventHumanUtterance, Payload: types.OutputLine{Role: "user", Text: text, Source: "server-stt"}})
		}
	}
}

func isExpectedSpeechStreamClose(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
		return true
	}
	return status.Code(err) == codes.Canceled
}

func (s *stage) sendSpeechAudio(audio []byte) {
	if len(audio) == 0 {
		return
	}
	s.mu.Lock()
	stream := s.speechStream
	s.mu.Unlock()
	if stream == nil {
		return
	}
	for start := 0; start < len(audio); start += speechAudioChunkBytes {
		end := min(start+speechAudioChunkBytes, len(audio))
		if (end-start)%2 != 0 {
			end--
		}
		if end <= start {
			continue
		}
		req := &speechpb.StreamingRecognizeRequest{
			StreamingRequest: &speechpb.StreamingRecognizeRequest_Audio{
				Audio: audio[start:end],
			},
		}
		if err := stream.Send(req); err != nil {
			log.Printf("stt: speech audio send error: %v", err)
			s.mu.Lock()
			if s.speechStream == stream {
				s.stopSpeechLocked()
			}
			s.mu.Unlock()
			return
		}
	}
}

func (s *stage) scheduleSpeechStop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelSpeechStopLocked()
	s.speechTimer = time.AfterFunc(sttStopDelay, func() {
		s.mu.Lock()
		s.closeSpeechSendLocked()
		s.mu.Unlock()
	})
}

func (s *stage) cancelSpeechStopLocked() {
	if s.speechTimer == nil {
		return
	}
	s.speechTimer.Stop()
	s.speechTimer = nil
}

func (s *stage) stopSpeechLocked() {
	s.cancelSpeechStopLocked()
	s.closeSpeechSendLocked()
	if s.speechCancel != nil {
		s.speechCancel()
		s.speechCancel = nil
	}
}

func (s *stage) closeSpeechSendLocked() {
	s.cancelSpeechStopLocked()
	if s.speechStream != nil {
		_ = s.speechStream.CloseSend()
		s.speechStream = nil
	}
}

func recognizerPath(projectID, recognizer string) string {
	id := strings.TrimSpace(recognizer)
	if id == "" {
		id = "_"
	}
	return fmt.Sprintf("projects/%s/locations/%s/recognizers/%s", strings.TrimSpace(projectID), speechRegion, id)
}
