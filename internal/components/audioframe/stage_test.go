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
	st.Upstream <- types.EventPlayableSpeech{PlayableSpeech: types.PlayableSpeech{GenerationID: 3, SequenceID: "item/1", Text: "こんにちは", Audio: base64.StdEncoding.EncodeToString(make([]byte, 2000))}}
	for i := 0; i < 3; i++ {
		select {
		case e := <-st.Downstream:
			p := e.(types.EventAudioFrame).AudioFrame
			if p.GenerationID != 3 || p.SegmentID != "item/1" || p.Sequence != i || p.Final != (i == 2) {
				t.Fatal(p)
			}
			if (i == 0 || i == 2) && p.Text != "こんにちは" {
				t.Fatal("先頭と末尾のフレームに読み上げ区間の本文が必要です")
			}
			if i == 1 && p.Text != "" {
				t.Fatal("中間フレームに本文を重複させません")
			}
			if i == 2 && p.Duration != time.Second*80/48000 {
				t.Fatal(p)
			}
			if want := map[bool]time.Duration{true: time.Second * 2000 / 48000, false: 0}[i == 0]; p.SegmentDuration != want {
				t.Fatalf("SegmentDuration = %v, want %v", p.SegmentDuration, want)
			}
		case <-ctx.Done():
			t.Fatal("タイムアウト")
		}
	}
}
