package audioframe

import (
	"context"
	"encoding/base64"
	"github.com/tetetratra/smart-speaker/internal/types"
	"testing"
	"time"
)

func TestFramesPreserveIdentityAndPartialTail(t *testing.T) {
	st := NewStage()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go st.Run(ctx)
	st.Upstream <- types.Event{Payload: types.PlayableSpeech{GenerationID: 3, SequenceID: "item/1", Text: "こんにちは", Audio: base64.StdEncoding.EncodeToString(make([]byte, 1000))}}
	for i := 0; i < 2; i++ {
		select {
		case e := <-st.Downstream:
			p := e.Payload.(types.AudioFrame)
			if p.GenerationID != 3 || p.SegmentID != "item/1" || p.Sequence != i || p.Final != (i == 1) {
				t.Fatal(p)
			}
			if i == 1 && (p.Text != "こんにちは" || p.Duration != time.Second*40/48000) {
				t.Fatal(p)
			}
		case <-ctx.Done():
			t.Fatal("タイムアウト")
		}
	}
}
