package rtcout

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/tetetratra/smart-speaker/internal/components/generationfilter"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/types"
)

type channelWriter struct{ samples chan media.Sample }

func (w channelWriter) WriteSample(sample media.Sample) error { w.samples <- sample; return nil }

func TestOutputDropsOldFramesImmediatelyBeforeWriting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	store := generation.NewStore()
	store.Next()
	node, err := NewStage(Config{})
	if err != nil {
		t.Fatal(err)
	}
	generationfilter.Attach(node, store)
	go node.Run(ctx)
	samples := make(chan media.Sample, 10)
	node.Upstream <- types.Event{Payload: types.RTCPeerOutputSink{PeerID: "peer", Writer: channelWriter{samples}, Connected: true}}
	frame := types.AudioFrame{ItemID: "old", GenerationID: 1, SampleRate: 24000, Audio: base64.StdEncoding.EncodeToString(make([]byte, 960))}
	node.Upstream <- types.Event{Kind: types.EventAudioFrame, Payload: frame}
	select {
	case <-samples:
	case <-ctx.Done():
		t.Fatal("最初の音声が届かなかった")
	}
	// 履歴通知の受け取り待ち中に割り込み、その後届く旧世代を拒否する。
	store.Next()
	<-node.Downstream
	node.Upstream <- types.Event{Kind: types.EventAudioFrame, Payload: frame}
	frame.GenerationID = 2
	frame.ItemID = "new"
	node.Upstream <- types.Event{Kind: types.EventAudioFrame, Payload: frame}
	select {
	case e := <-node.Downstream:
		if e.Kind != types.EventAudioSent || e.Payload.(types.AudioFrame).ItemID != "new" {
			t.Fatal("旧世代を送信した", e)
		}
	case <-ctx.Done():
		t.Fatal("新世代が送信されなかった")
	}
	if len(samples) != 1 {
		t.Fatalf("旧世代が音声トラックへ流れた: %d", len(samples))
	}
}
