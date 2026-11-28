// Package conversationflow は会話の流れを決める。会話の記録（conversationrecorder が保存したもの）を受け取り、
// ユーザーの発話なら振り分け（decision）を頼み、判定結果に応じて頭脳AI（brain）に任せたり、realtime-response に応答を頼んだりする。
// 後から届いた結果や知らせで応答を求めるもの（ModelShouldRespond）は、頭脳AIに回す。
package conversationflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/types"
)

type Config struct {
	Generation *generation.Store
}

// turn は、ユーザーの発話1回への対応。判定待ちか、頭脳AIの最初の話す内容を待っている間だけ持つ。
type turn struct {
	id         string
	generation types.GenerationID
	// brainRequest は、頭脳AIに任せたときの依頼の ID。判定待ちの間は空。
	brainRequest string
}

type stage struct {
	node *graph.Stage
	cfg  Config
	turn *turn
	// held は、ターンの途中に届いた、ターンの外の応答依頼。ターンが終わったら届いた順に流す。
	held []types.ResponseRequest
}

func NewStage(cfg Config) (*graph.Stage, error) {
	if cfg.Generation == nil {
		return nil, fmt.Errorf("conversationflow: 世代のStoreが必要")
	}
	s := &stage{cfg: cfg}
	s.node = &graph.Stage{Name: "conversationflow", Upstream: make(chan types.Event), Downstream: make(chan types.Event), Run: s.run, CloseFn: func() error { return nil }}
	return s.node, nil
}

func (s *stage) run(ctx context.Context) {
	defer close(s.node.Downstream)
	for {
		e, ok := s.node.Receive(ctx)
		if !ok {
			return
		}
		s.handle(ctx, e)
	}
}

func (s *stage) emit(ctx context.Context, e types.Event) {
	select {
	case s.node.Downstream <- e:
	case <-ctx.Done():
	}
}

func (s *stage) handle(ctx context.Context, e types.Event) {
	switch e := e.(type) {
	case types.EventConversationInput:
		s.record(ctx, e.ConversationInput)
	case types.EventDecision:
		s.decided(ctx, e.Decision)
	}
}

func (s *stage) record(ctx context.Context, input types.ConversationInput) {
	record := input.Record
	switch {
	case !input.ModelAlreadyKnows && input.ModelShouldRespond:
		// 後から届いた結果や知らせへの対応は、頭脳AIが会話の履歴を読んで考える。
		s.emit(ctx, types.EventBrainRequest{BrainRequest: types.BrainRequest{RequestID: uuid.NewString()}})
	case !input.ModelAlreadyKnows:
		// 応答を求めない記録は、参考情報として Realtime の会話に入れるだけ。
		s.emit(ctx, types.EventConversationInput{ConversationInput: input})
	case record.Source == types.BrainSource:
		s.brainSaid(ctx, record)
	case record.Role == types.RoleUser && record.Source == types.RealtimeSource && strings.TrimSpace(record.Text) != "":
		s.startTurn(ctx, record)
	}
}

// startTurn はユーザーの発話でターンを始め、振り分けを頼む。前のターンが残っていても、新しい発話で置き換える。
func (s *stage) startTurn(ctx context.Context, record types.ConversationRecord) {
	// 文字起こしが届く前にユーザーがまた話し始めていれば、その発話の記録でターンを始める。
	if record.GenerationID != 0 && !s.cfg.Generation.IsCurrent(record.GenerationID) {
		return
	}
	s.turn = &turn{id: uuid.NewString(), generation: record.GenerationID}
	s.emit(ctx, types.EventDecisionRequest{DecisionRequest: types.DecisionRequest{TurnID: s.turn.id, GenerationID: record.GenerationID}})
}

// current は、判定や話す内容が今のターンのもので、まだ世代が進んでいないかを返す。
func (s *stage) current(t *turn) bool {
	return t != nil && t == s.turn && (t.generation == 0 || s.cfg.Generation.IsCurrent(t.generation))
}

func (s *stage) decided(ctx context.Context, d types.Decision) {
	t := s.turn
	if t == nil || t.id != d.TurnID || t.brainRequest != "" {
		return
	}
	if !s.current(t) {
		s.endTurn(ctx)
		return
	}
	switch d.Choice {
	case types.DecisionRespond:
		s.emit(ctx, types.EventResponseRequest{ResponseRequest: types.ResponseRequest{GenerationID: t.generation}})
		s.endTurn(ctx)
	case types.DecisionDelegate:
		t.brainRequest = uuid.NewString()
		s.emit(ctx, types.EventBrainRequest{BrainRequest: types.BrainRequest{RequestID: t.brainRequest, GenerationID: t.generation}})
		// 頭脳AIが考えている間の沈黙を埋める。頭脳AIの話す内容は、この応答の後に並ぶ。
		s.emit(ctx, types.EventResponseRequest{ResponseRequest: types.ResponseRequest{GenerationID: t.generation, Notice: firstResponseNotice}})
	default:
		// もう少し聞く・応答しない。どちらも何もせず、次の発話を待つ。
		s.endTurn(ctx)
	}
}

// brainSaid は頭脳AIの話す内容を伝えさせる。今のターンで任せた依頼の最初の答えならターンの中、それ以外はターンの外の発話として扱う。
func (s *stage) brainSaid(ctx context.Context, record types.ConversationRecord) {
	requestID, _ := record.Metadata[types.BrainRequestMetadataKey].(string)
	silent, _ := record.Metadata[types.BrainSilentMetadataKey].(bool)
	t := s.turn
	if t != nil && t.brainRequest != "" && t.brainRequest == requestID {
		// ユーザーが話し足していたら、古い答えは伝えない。
		if s.current(t) && !silent {
			s.emit(ctx, types.EventResponseRequest{ResponseRequest: types.ResponseRequest{GenerationID: t.generation, Notice: brainNotice(record.Text)}})
		}
		s.endTurn(ctx)
		return
	}
	// ユーザーの発話から任せた依頼（世代つき）の答えなのに、今のターンのものでなければ、ユーザーが話し足してターンが置き換わったあとに届いた古い答え。伝えない。
	if silent || record.GenerationID != 0 {
		return
	}
	// ターンの外の発話（後から届いた結果への対応など）は、ユーザーが話しても捨てない。ターンの途中なら、終わるまでためておく。
	req := types.ResponseRequest{Notice: brainNotice(record.Text), NewGeneration: true}
	if s.turn != nil {
		s.held = append(s.held, req)
		return
	}
	s.emit(ctx, types.EventResponseRequest{ResponseRequest: req})
}

// endTurn はターンを終え、ためていた応答依頼を流す。
func (s *stage) endTurn(ctx context.Context) {
	s.turn = nil
	held := s.held
	s.held = nil
	for _, req := range held {
		s.emit(ctx, types.EventResponseRequest{ResponseRequest: req})
	}
}

// firstResponseNotice と brainNotice は、応答のたびに Realtime の会話に足す知らせ。
// 伝え方の指示は Realtime のシステムプロンプト（「頭脳AIとの分担」）に1回だけ書き、ここには毎回変わる中身だけを入れる。
const firstResponseNotice = "頭脳AIが対応中。"

func brainNotice(text string) string {
	return "## 頭脳AIが用意した内容\n" + text
}
