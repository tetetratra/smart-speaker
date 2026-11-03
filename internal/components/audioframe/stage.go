package audioframe

import (
	"context"
	"encoding/base64"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/types"
	"log"
	"time"
)

// NewStage は24kHz・モノラルPCM16を20ms単位へ変換する。待機と世代判定は接続側に任せる。
func NewStage() *graph.Stage {
	st := &graph.Stage{Name: "audioframe", Upstream: make(chan types.Event), Downstream: make(chan types.Event), CloseFn: func() error { return nil }}
	st.Run = func(ctx context.Context) {
		defer close(st.Downstream)
		emit := func(p types.AudioFrame) bool {
			select {
			case <-ctx.Done():
				return false
			case st.Downstream <- types.Event{Kind: types.EventAudioFrame, Payload: p}:
				return true
			}
		}
		for {
			e, ok := st.Receive(ctx)
			if !ok {
				return
			}
			switch p := e.Payload.(type) {
			case types.AgentTimelineEnd:
				if !emit(types.AudioFrame{GenerationID: p.GenerationID, ItemID: p.ItemID, End: true}) {
					return
				}
			case types.PlayableSpeech:
				pcm, err := base64.StdEncoding.DecodeString(p.Audio)
				if err != nil || len(pcm)%2 != 0 {
					log.Printf("audioframe: invalid PCM")
					continue
				}
				for offset := 0; offset < len(pcm); offset += 960 {
					end := min(offset+960, len(pcm))
					frame := types.AudioFrame{GenerationID: p.GenerationID, SegmentID: p.SequenceID, ItemID: p.OriginalTimeline.ItemID, Sequence: offset / 960, Audio: base64.StdEncoding.EncodeToString(pcm[offset:end]), SampleRate: 24000, Final: end == len(pcm), Duration: time.Duration(end-offset) * time.Second / 48000}
					if frame.Final {
						frame.Text = p.Text
					}
					if !emit(frame) {
						return
					}
				}
			}
		}
	}
	return st
}
