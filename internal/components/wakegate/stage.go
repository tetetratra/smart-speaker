package wakegate

import (
	"context"
	"encoding/binary"
	"log"
	"math"
	"time"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/wakemode"
	"github.com/tetetratra/smart-speaker/internal/types"
)

type Config struct {
	Store           *wakemode.Store
	STT             Transcriber
	EnergyThreshold float64
}

// levelLogIntervalは、呼びかけ必須モード中にマイクの音量を記録する間隔。
const levelLogInterval = 5 * time.Second

type peerBuffer struct {
	revision, epoch     uint64
	pcm, pre            []byte
	rate, quiet, voiced int
	overflow            bool
	// peakは区切り中の発話の最大音量。levelPeakとlevelSinceは定期的な音量ログ用。
	peak, levelPeak float64
	levelSince      time.Time
}
type job struct {
	peer            string
	revision, epoch uint64
	pcm             []byte
	rate            int
	queuedAt        time.Time
}
type result struct {
	job
	transcript Transcript
	err        error
}

func NewStage(cfg Config) *graph.Stage {
	node := &graph.Stage{Name: "wakegate", Upstream: make(chan types.Event), Downstream: make(chan types.Event), CloseFn: func() error { return nil }}
	if cfg.EnergyThreshold <= 0 {
		cfg.EnergyThreshold = 0.012
	}
	node.Run = func(ctx context.Context) {
		defer close(node.Downstream)
		emit := func(e types.Event) {
			select {
			case node.Downstream <- e:
			case <-ctx.Done():
			}
		}
		jobs := make(chan job, 2)
		results := make(chan result, 1)
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case j := <-jobs:
					runCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
					var tr Transcript
					var err error
					started := time.Now()
					if cfg.STT != nil {
						tr, err = cfg.STT.Transcribe(runCtx, j.pcm, j.rate)
					} else {
						err = context.Canceled
					}
					cancel()
					log.Printf("wakegate: STT完了 peer=%s 待ち=%s 処理=%s err=%v", j.peer, started.Sub(j.queuedAt).Round(time.Millisecond), time.Since(started).Round(time.Millisecond), err)
					select {
					case results <- result{job: j, transcript: tr, err: err}:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
		peers := map[string]*peerBuffer{}
		var epoch uint64
		for {
			select {
			case <-ctx.Done():
				return
			case r := <-results:
				b := peers[r.peer]
				if b == nil || b.epoch != r.epoch || cfg.Store == nil {
					log.Printf("wakegate: 端末の再接続で認識結果を破棄 peer=%s text=%q", r.peer, r.transcript.Text)
					continue
				}
				mode := cfg.Store.Snapshot(time.Now())
				if !mode.Required || mode.Revision != r.revision {
					log.Printf("wakegate: 設定の変更で認識結果を破棄 peer=%s text=%q", r.peer, r.transcript.Text)
					continue
				}
				// 呼びかけの確定前に区切った発話は、会話中へ移った後に結果が届いても受け付ける。
				d := cfg.Store.Evaluate(r.transcript.Text, r.transcript.Confidence, r.revision, r.err != nil, time.Now())
				// 原因調査用に、STTがどう書き起こしたかと判定を残す。呼びかけのない発話の内容も含む。
				log.Printf("wakegate: 判定 peer=%s text=%q confidence=%.3f similarity=%.3f matched=%t accepted=%t wake_only=%t awake=%t err=%v", r.peer, r.transcript.Text, d.Confidence, d.Similarity, d.Matched, d.Accepted, d.WakeOnly, mode.Awake, r.err)
				if d.Accepted {
					emit(types.Event{Kind: types.EventLocalWakeUtterance, Payload: types.LocalWakeUtterance{PeerID: r.peer, Text: r.transcript.Text, Revision: r.revision, Confidence: d.Confidence, WakeOnly: d.WakeOnly}})
				}
			case e, ok := <-node.Upstream:
				if !ok {
					return
				}
				switch p := e.Payload.(type) {
				case types.RTCPeerOutputSink:
					if p.Connected {
						epoch++
						peers[p.PeerID] = &peerBuffer{epoch: epoch}
					} else {
						delete(peers, p.PeerID)
					}
					emit(e)
				case types.RTCPeerAudioFrame:
					b := peers[p.PeerID]
					if b == nil {
						continue
					}
					if cfg.Store == nil {
						emit(e)
						continue
					}
					mode := cfg.Store.Snapshot(time.Now())
					// 不要モードと呼びかけ後の会話中は、ローカルSTTを通さず音声をそのまま送る。
					if !mode.Gated() {
						b.pcm = nil
						b.pre = nil
						b.quiet = 0
						b.voiced = 0
						b.overflow = false
						emit(e)
						continue
					}
					if p.Channels != 1 || p.SampleRate < 8000 || p.SampleRate > 96000 || len(p.PCM) == 0 || len(p.PCM)%2 != 0 {
						continue
					}
					if b.revision != mode.Revision || b.rate != p.SampleRate {
						savedEpoch := b.epoch
						*b = peerBuffer{epoch: savedEpoch, revision: mode.Revision, rate: p.SampleRate}
					}
					// 無音を区切りにし、最大30秒。長すぎる連続音声は途中から新しい依頼にしない。
					level := rms(p.PCM)
					voiced := level >= cfg.EnergyThreshold
					b.levelPeak = max(b.levelPeak, level)
					if b.levelSince.IsZero() {
						b.levelSince = time.Now()
					} else if time.Since(b.levelSince) >= levelLogInterval {
						log.Printf("wakegate: 音量 peer=%s 直近%sの最大RMS=%.4f しきい値=%.4f", p.PeerID, levelLogInterval, b.levelPeak, cfg.EnergyThreshold)
						b.levelPeak = 0
						b.levelSince = time.Now()
					}
					if len(b.pcm) == 0 && !voiced && !b.overflow {
						b.pre = append(b.pre, p.PCM...)
						limit := b.rate * 2 / 5
						if len(b.pre) > limit {
							b.pre = append([]byte(nil), b.pre[len(b.pre)-limit:]...)
						}
						continue
					}
					if voiced {
						b.quiet = 0
						b.voiced += len(p.PCM) / 2
						b.peak = max(b.peak, level)
					} else {
						b.quiet += len(p.PCM) / 2
					}
					if !b.overflow {
						if len(b.pcm) == 0 {
							b.pcm = append(b.pcm, b.pre...)
							b.pre = nil
						}
						b.pcm = append(b.pcm, p.PCM...)
						if len(b.pcm) > b.rate*2*30 {
							log.Printf("wakegate: 30秒を超える連続音声を無視 peer=%s", p.PeerID)
							b.pcm = nil
							b.overflow = true
						}
					}
					if b.quiet >= b.rate*8/10 {
						length := time.Duration(len(b.pcm)/2) * time.Second / time.Duration(b.rate)
						voicedLength := time.Duration(b.voiced) * time.Second / time.Duration(b.rate)
						if !b.overflow && b.voiced >= b.rate/5 {
							j := job{peer: p.PeerID, revision: mode.Revision, epoch: b.epoch, rate: b.rate, pcm: b.pcm, queuedAt: time.Now()}
							select {
							case jobs <- j:
								log.Printf("wakegate: 発話を区切りSTTへ送信 peer=%s 長さ=%s 有声=%s 最大RMS=%.4f", p.PeerID, length.Round(time.Millisecond), voicedLength.Round(time.Millisecond), b.peak)
							default:
								log.Printf("wakegate: STTの待ち行列が満杯のため発話を破棄 peer=%s 長さ=%s", p.PeerID, length.Round(time.Millisecond))
								cfg.Store.Evaluate("", 0, mode.Revision, true, time.Now())
							}
						} else if !b.overflow {
							log.Printf("wakegate: 有声部分が200ms未満のため無視 peer=%s 有声=%s 最大RMS=%.4f", p.PeerID, voicedLength.Round(time.Millisecond), b.peak)
						}
						b.peak = 0
						b.pcm = nil
						b.pre = nil
						b.voiced = 0
						b.quiet = 0
						b.overflow = false
					}
				}
			}
		}
	}
	return node
}
func rms(pcm []byte) float64 {
	var sum float64
	for i := 0; i+1 < len(pcm); i += 2 {
		v := float64(int16(binary.LittleEndian.Uint16(pcm[i:]))) / 32768
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(pcm)/2))
}
