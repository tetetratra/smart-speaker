package rtcout

import (
	"context"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/types"
	opus "gopkg.in/hraban/opus.v2"
)

const (
	webrtcSampleRate = 48000
	opusFrameMs      = 20
)

type Config struct{}

// NewStage は送信時刻の調整を終えた一フレームだけを圧縮・送信する。
// 待機音声を内部に蓄積せず、受け取り直後の世代フィルターを有効にする。
func NewStage(cfg Config) (*graph.Stage, error) {
	s := &stage{sinks: map[string]*peerSink{}, gains: map[string]float64{}}
	s.node = &graph.Stage{Name: "rtcout", Upstream: make(chan types.Event), Downstream: make(chan types.Event)}
	s.node.Run = s.run
	s.node.CloseFn = func() error { close(s.node.Upstream); return nil }
	return s.node, nil
}

type stage struct {
	node  *graph.Stage
	sinks map[string]*peerSink
	gains map[string]float64
}

type peerSink struct {
	writer       types.RTCPeerOutputWriter
	encoder      *opus.Encoder
	opusChannels int
	gain         float64
}

func (s *stage) run(ctx context.Context) {
	defer close(s.node.Downstream)
	for {
		e, ok := s.node.Receive(ctx)
		if !ok {
			return
		}
		switch p := e.Payload.(type) {
		case types.RTCPeerOutputSink:
			s.handlePeerOutputSink(p)
		case types.RTCSignal:
			s.handleVolume(p)
		case types.AudioFrame:
			if !s.sendFrame(p) {
				continue
			}
			// 履歴の送信実績はWebSocket通知の成功ではなく、音声トラックへの書き込み成功で記録する。
			select {
			case <-ctx.Done():
				return
			case s.node.Downstream <- types.Event{Kind: types.EventAudioSent, Payload: p}:
			}
		}
	}
}
