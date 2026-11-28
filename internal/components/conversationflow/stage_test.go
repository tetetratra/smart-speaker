package conversationflow

import (
	"context"
	"strings"
	"testing"

	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/states/generation"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func testStage() *stage {
	gen := generation.NewStore()
	gen.Next()
	return &stage{cfg: Config{Generation: gen}, node: &graph.Stage{Downstream: make(chan types.Event, 20)}}
}

func (s *stage) userSaid(text string) types.EventDecisionRequest {
	s.handle(context.Background(), types.EventConversationInput{ConversationInput: types.ConversationInput{
		Record: types.ConversationRecord{Role: types.RoleUser, Source: types.RealtimeSource, Text: text, GenerationID: s.cfg.Generation.Current()}, ModelAlreadyKnows: true}})
	return (<-s.node.Downstream).(types.EventDecisionRequest)
}

func (s *stage) brainAnswered(requestID, text string, silent bool) {
	metadata := map[string]any{types.BrainRequestMetadataKey: requestID}
	if silent {
		metadata[types.BrainSilentMetadataKey] = true
	}
	s.handle(context.Background(), types.EventConversationInput{ConversationInput: types.ConversationInput{
		Record: types.ConversationRecord{Role: types.RoleBackground, Source: types.BrainSource, Text: text, Metadata: metadata}, ModelAlreadyKnows: true, UserAlreadyKnows: true}})
}

func (s *stage) next(t *testing.T) types.Event {
	t.Helper()
	select {
	case e := <-s.node.Downstream:
		return e
	default:
		t.Fatal("イベントが出ていません")
		return nil
	}
}

func (s *stage) none(t *testing.T) {
	t.Helper()
	select {
	case e := <-s.node.Downstream:
		t.Fatal("不要なイベントです", e)
	default:
	}
}

// 「Realtime がそのまま返す」なら、その発話の世代で応答を頼み、ターンを終える。
func TestRespondDecisionRequestsResponseAndEndsTurn(t *testing.T) {
	s := testStage()
	req := s.userSaid("おはよう")
	if req.GenerationID != s.cfg.Generation.Current() {
		t.Fatal(req)
	}
	s.handle(context.Background(), types.EventDecision{Decision: types.Decision{TurnID: req.TurnID, GenerationID: req.GenerationID, Choice: types.DecisionRespond}})
	res := s.next(t).(types.EventResponseRequest)
	if res.GenerationID != req.GenerationID || res.Notice != "" || res.NewGeneration {
		t.Fatal(res)
	}
	if s.turn != nil {
		t.Fatal("ターンが終わっていません")
	}
}

// 頭脳AIに任せたら、一次応答を頼み、最初の話す内容が届いたら本回答を頼んでターンを終える。
func TestDelegateDecisionAsksBrainAndSendsFirstResponseThenAnswer(t *testing.T) {
	s := testStage()
	req := s.userSaid("明日の予定も見て、どっちがいい？")
	s.handle(context.Background(), types.EventDecision{Decision: types.Decision{TurnID: req.TurnID, GenerationID: req.GenerationID, Choice: types.DecisionDelegate}})
	brain := s.next(t).(types.EventBrainRequest)
	if brain.GenerationID != req.GenerationID {
		t.Fatal(brain)
	}
	first := s.next(t).(types.EventResponseRequest)
	if first.Notice != firstResponseNotice || first.GenerationID != req.GenerationID {
		t.Fatal(first)
	}
	s.brainAnswered(brain.RequestID, "明後日がよい", false)
	answer := s.next(t).(types.EventResponseRequest)
	if !strings.Contains(answer.Notice, "明後日がよい") || answer.GenerationID != req.GenerationID || answer.NewGeneration {
		t.Fatal(answer)
	}
	if s.turn != nil {
		t.Fatal("ターンが終わっていません")
	}
}

// ユーザーが話し足して世代が進んだら、古い判定・話す内容は伝えない。
func TestStaleDecisionAndAnswerAreDropped(t *testing.T) {
	s := testStage()
	req := s.userSaid("えっと")
	s.cfg.Generation.Next()
	s.handle(context.Background(), types.EventDecision{Decision: types.Decision{TurnID: req.TurnID, GenerationID: req.GenerationID, Choice: types.DecisionRespond}})
	s.none(t)

	req = s.userSaid("電気消して")
	s.handle(context.Background(), types.EventDecision{Decision: types.Decision{TurnID: req.TurnID, GenerationID: req.GenerationID, Choice: types.DecisionDelegate}})
	brain := s.next(t).(types.EventBrainRequest)
	s.next(t)
	s.cfg.Generation.Next()
	s.brainAnswered(brain.RequestID, "消したよ", false)
	s.none(t)
}

// もう少し聞く・応答しない、と頭脳AIが何も話さないと判断したときは、何も頼まずにターンを終える。
func TestWaitIgnoreAndSilentAnswerEndTurnWithoutResponse(t *testing.T) {
	for _, choice := range []types.DecisionChoice{types.DecisionWait, types.DecisionIgnore} {
		s := testStage()
		req := s.userSaid("うーん")
		s.handle(context.Background(), types.EventDecision{Decision: types.Decision{TurnID: req.TurnID, GenerationID: req.GenerationID, Choice: choice}})
		s.none(t)
		if s.turn != nil {
			t.Fatal(choice)
		}
	}
	s := testStage()
	req := s.userSaid("テレビの声")
	s.handle(context.Background(), types.EventDecision{Decision: types.Decision{TurnID: req.TurnID, GenerationID: req.GenerationID, Choice: types.DecisionDelegate}})
	brain := s.next(t).(types.EventBrainRequest)
	s.next(t)
	s.brainAnswered(brain.RequestID, "[何も話さない]", true)
	s.none(t)
	if s.turn != nil {
		t.Fatal("ターンが終わっていません")
	}
}

// 応答を求める記録は頭脳AIに回し、ターンの外の話す内容は、ターン中ならためて、ターンが終わってから新しい世代で頼む。
func TestOutOfTurnAnswerIsHeldDuringTurn(t *testing.T) {
	s := testStage()
	s.handle(context.Background(), types.EventConversationInput{ConversationInput: types.ConversationInput{Record: types.ConversationRecord{Role: types.RoleBackground, Source: "claude_code", Text: "終わった"}, ModelShouldRespond: true}})
	background := s.next(t).(types.EventBrainRequest)
	if background.GenerationID != 0 {
		t.Fatal(background)
	}
	req := s.userSaid("おはよう")
	s.brainAnswered(background.RequestID, "テストが直ったよ", false)
	s.none(t)
	// ユーザーが話しても、ターンの外の話す内容は捨てない。
	s.cfg.Generation.Next()
	req = s.userSaid("ありがとう")
	s.handle(context.Background(), types.EventDecision{Decision: types.Decision{TurnID: req.TurnID, GenerationID: req.GenerationID, Choice: types.DecisionRespond}})
	if res := s.next(t).(types.EventResponseRequest); res.NewGeneration {
		t.Fatal("ターンの応答より先に、ためた依頼を流しました", res)
	}
	held := s.next(t).(types.EventResponseRequest)
	if !held.NewGeneration || !strings.Contains(held.Notice, "テストが直ったよ") {
		t.Fatal(held)
	}
}

// 応答を求めない記録は参考情報として応答生成側へ渡し、空の文字起こしではターンを始めない。
func TestContextRecordIsForwardedAndEmptyUtteranceIsIgnored(t *testing.T) {
	s := testStage()
	input := types.ConversationInput{Record: types.ConversationRecord{Role: types.RoleSystem, Source: "user_location", Text: "外出した"}}
	s.handle(context.Background(), types.EventConversationInput{ConversationInput: input})
	if e := s.next(t).(types.EventConversationInput); e.Record.Text != "外出した" {
		t.Fatal(e)
	}
	s.handle(context.Background(), types.EventConversationInput{ConversationInput: types.ConversationInput{Record: types.ConversationRecord{Role: types.RoleUser, Source: types.RealtimeSource, Text: ""}, ModelAlreadyKnows: true}})
	s.handle(context.Background(), types.EventConversationInput{ConversationInput: types.ConversationInput{Record: types.ConversationRecord{Role: types.RoleAgent, Source: types.RealtimeSource, Text: "はい"}, ModelAlreadyKnows: true}})
	s.none(t)
}

// 頭脳AIが考えている間にユーザーが話し足してターンが置き換わったら、古いターンの答えは、あとで届いても伝えない。
func TestAnswerOfReplacedTurnIsDropped(t *testing.T) {
	s := testStage()
	req := s.userSaid("電気消して")
	s.handle(context.Background(), types.EventDecision{Decision: types.Decision{TurnID: req.TurnID, GenerationID: req.GenerationID, Choice: types.DecisionDelegate}})
	brain := s.next(t).(types.EventBrainRequest)
	s.next(t)
	s.cfg.Generation.Next()
	s.userSaid("やっぱりいいや")
	s.handle(context.Background(), types.EventConversationInput{ConversationInput: types.ConversationInput{
		Record: types.ConversationRecord{Role: types.RoleBackground, Source: types.BrainSource, Text: "消した", GenerationID: brain.GenerationID,
			Metadata: map[string]any{types.BrainRequestMetadataKey: brain.RequestID}}, ModelAlreadyKnows: true}})
	s.none(t)
	if len(s.held) != 0 {
		t.Fatal("古いターンの答えをためました", s.held)
	}
}
