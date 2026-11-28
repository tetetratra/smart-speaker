// Package automation は、予約ジョブをグラフに載せるコンポーネント。
// 時刻で動く予約に加えて、記録係（conversationrecorder）から届いた購読可能な記録で動く予約を動かす。
// 予約ジョブの実行結果（remind・ツールの呼び出しと結果・スクリプトの失敗）は、gateway を通さず記録係（conversationrecorder）へ直接送る。
package automation

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/tetetratra/smart-speaker/internal/eventqueue"
	"github.com/tetetratra/smart-speaker/internal/features/automations"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/types"
)

// Stage は予約ジョブのコンポーネント。
// スクリプトを動かす Runner はコンポーネントより先に作られるので、Stage を先に作って Deliver を渡す。
type Stage struct {
	node    *graph.Stage
	service *automations.Service
	// records は、届いた購読可能な記録の待機列。ジョブの実行中もグラフからの受け取りを止めないために使う。
	records *eventqueue.Queue[types.ConversationRecord]
	// outputs は、記録係（conversationrecorder）へ送る予約ジョブの実行結果の待機列。
	outputs *eventqueue.Queue[types.Event]
}

func New(service *automations.Service) *Stage {
	s := &Stage{service: service, records: eventqueue.New[types.ConversationRecord](nil), outputs: eventqueue.New[types.Event](nil)}
	s.node = &graph.Stage{Name: "automation", Upstream: make(chan types.Event), Downstream: make(chan types.Event), Run: s.run, CloseFn: func() error { return nil }}
	return s
}

// Stage はグラフに載せるコンポーネントを返す。
func (s *Stage) Stage() *graph.Stage { return s.node }

// Deliver は、予約ジョブの実行結果を記録係（conversationrecorder）へ送る待機列に積む。Runner に渡す。
// 予約ジョブの実行結果は購読可能にしない（自分の結果で発火し続けないように）。
func (s *Stage) Deliver(ctx context.Context, input types.ConversationInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	input.Subscribable = false
	s.outputs.Push(types.EventConversationInput{ConversationInput: input})
	return nil
}

// recordPayload は、記録で動いたスクリプトの event 変数に渡す記録の中身。
type recordPayload struct {
	Source    string         `json:"source"`
	Title     string         `json:"title"`
	Text      string         `json:"text"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
}

func (s *Stage) run(ctx context.Context) {
	defer close(s.node.Downstream)
	go func() {
		for {
			e, ok := s.node.Receive(ctx)
			if !ok {
				return
			}
			// 記録係（conversationrecorder）からは、会話に入る記録がすべて届く。購読可能なものだけを扱う。
			if input, ok := e.(types.EventConversationInput); ok && input.Subscribable {
				s.records.Push(input.Record)
			}
		}
	}()
	// 記録で動くジョブは、届いた順に1つずつ動かす。
	go func() {
		for {
			record, ok := s.records.Pop(ctx)
			if !ok {
				return
			}
			s.handle(ctx, record)
		}
	}()
	go s.service.Run(ctx)
	for {
		e, ok := s.outputs.Pop(ctx)
		if !ok {
			return
		}
		select {
		case s.node.Downstream <- e:
		case <-ctx.Done():
			return
		}
	}
}

func (s *Stage) handle(ctx context.Context, record types.ConversationRecord) {
	payload, err := json.Marshal(recordPayload{Source: record.Source, Title: record.Title, Text: record.Text, Metadata: record.Metadata, CreatedAt: record.CreatedAt})
	if err != nil {
		log.Printf("予約ジョブ: 記録の中身を JSON にできません source=%s: %v", record.Source, err)
		return
	}
	if err := s.service.HandleRecord(ctx, record.Source, payload, time.Now()); err != nil {
		log.Printf("予約ジョブ: 記録で動くジョブの実行 source=%s: %v", record.Source, err)
	}
}
