package rtc

import (
	"context"
	"io"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMeasureFrameEnergy(t *testing.T) {
	t.Run("無音フレームの場合", func(t *testing.T) {
		if got := measureFrameEnergy([]int16{0, 0, 0, 0}); got != 0 {
			t.Fatalf("expected 0, got %d", got)
		}
	})

	t.Run("一定振幅のフレームの場合", func(t *testing.T) {
		got := measureFrameEnergy([]int16{-10, 10, -10, 10})
		if got != 10 {
			t.Fatalf("expected 10, got %d", got)
		}
	})
}

func TestComputeAdaptiveSpeechThreshold(t *testing.T) {
	t.Run("履歴が十分にある場合", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		samples := []energySample{
			{energy: 80, capturedAt: now},
			{energy: 100, capturedAt: now},
			{energy: 120, capturedAt: now},
		}

		got := computeAdaptiveSpeechThreshold(samples)
		if got != 150 {
			t.Fatalf("expected 150, got %d", got)
		}
	})

	t.Run("中央値プラス50が最低しきい値未満の場合", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		samples := []energySample{
			{energy: 0, capturedAt: now},
			{energy: 0, capturedAt: now},
			{energy: 0, capturedAt: now},
		}

		got := computeAdaptiveSpeechThreshold(samples)
		if got != adaptiveVADMinThreshold {
			t.Fatalf("expected %d, got %d", adaptiveVADMinThreshold, got)
		}
	})
}

func TestAppendEnergySample(t *testing.T) {
	t.Run("1分より古い履歴を破棄すること", func(t *testing.T) {
		now := time.Date(2026, 1, 1, 12, 1, 0, 0, time.UTC)
		samples := []energySample{
			{energy: 10, capturedAt: now.Add(-adaptiveVADHistoryWindow - time.Second)},
			{energy: 20, capturedAt: now.Add(-30 * time.Second)},
		}

		got := appendEnergySample(samples, now, 30)
		if len(got) != 2 {
			t.Fatalf("expected 2 samples after pruning, got %d", len(got))
		}
		if got[0].energy != 20 || got[1].energy != 30 {
			t.Fatalf("unexpected samples: %#v", got)
		}
	})
}

func TestIsSpeechFrame(t *testing.T) {
	t.Run("静かな環境で小声入力の場合", func(t *testing.T) {
		if !isSpeechFrame(55, adaptiveVADMinThreshold) {
			t.Fatal("expected quiet voice to be treated as speech")
		}
	})

	t.Run("現在のしきい値未満のノイズの場合", func(t *testing.T) {
		if isSpeechFrame(40, 60) {
			t.Fatal("expected noise below threshold to be treated as non-speech")
		}
	})
}

func TestIsExpectedSpeechStreamClose(t *testing.T) {
	t.Run("context canceled", func(t *testing.T) {
		if !isExpectedSpeechStreamClose(context.Canceled) {
			t.Fatal("context.Canceled should be expected")
		}
	})

	t.Run("io EOF", func(t *testing.T) {
		if !isExpectedSpeechStreamClose(io.EOF) {
			t.Fatal("io.EOF should be expected")
		}
	})

	t.Run("grpc canceled", func(t *testing.T) {
		err := status.Error(codes.Canceled, "context canceled")
		if !isExpectedSpeechStreamClose(err) {
			t.Fatal("grpc codes.Canceled should be expected")
		}
	})

	t.Run("other grpc error", func(t *testing.T) {
		err := status.Error(codes.Unavailable, "temporary unavailable")
		if isExpectedSpeechStreamClose(err) {
			t.Fatal("codes.Unavailable should not be expected")
		}
	})
}
