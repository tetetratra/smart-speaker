package realtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tetetratra/smart-speaker/internal/types"
)

func request(s *stage, notice string) {
	s.handle(context.Background(), types.EventResponseRequest{ResponseRequest: types.ResponseRequest{GenerationID: s.cfg.Generation.Current(), Notice: notice}})
}

func finishResponse(s *stage, id string, metadata any) {
	serverEventForTest(s, map[string]any{"type": "response.created", "response": map[string]any{"id": id, "metadata": metadata}})
	serverEventForTest(s, map[string]any{"type": "response.done", "response": map[string]any{"id": id, "status": "completed"}})
}

// 生成中に届いた応答依頼は、届いた順にためておき、前の応答が終わってから1件ずつ送る（一次応答のあとに本回答）。
func TestResponseRequestsAreSentOneByOneInOrder(t *testing.T) {
	s := newTestStage(t)
	request(s, "一次応答の指示")
	first, notice := popResponse(t, s)
	if !strings.Contains(notice, "一次応答の指示") {
		t.Fatal(notice)
	}
	request(s, "本回答の指示")
	request(s, "次の指示")
	// 一次応答の生成が終わるまでは送らない。
	noCommand(t, s)
	finishResponse(s, "first", first["metadata"])
	second, notice := popResponse(t, s)
	if !strings.Contains(notice, "本回答の指示") {
		t.Fatal("届いた順に送っていません", notice)
	}
	noCommand(t, s)
	finishResponse(s, "second", second["metadata"])
	if _, notice = popResponse(t, s); !strings.Contains(notice, "次の指示") {
		t.Fatal(notice)
	}
}

// ユーザーが話し足して世代が進んだら、前のターンの応答依頼は送らない。
func TestStaleResponseRequestIsDropped(t *testing.T) {
	s := newTestStage(t)
	s.active = "existing"
	request(s, "古いターンの指示")
	s.controller.Start("user2")
	s.active = ""
	s.maybeRespond(context.Background())
	noCommand(t, s)
	if len(s.pendingRequests) != 0 {
		t.Fatal(s.pendingRequests)
	}
}

// ターンの外の応答依頼は、ユーザーが話しても捨てず、読み上げ中の応答が終わってから新しい世代で送る。
func TestOutOfTurnRequestWaitsForPlaybackAndStartsNewGeneration(t *testing.T) {
	s := newTestStage(t)
	current := s.cfg.Generation.Current()
	s.items["playing"] = &assistantItem{generation: current}
	s.pendingOutput["playing"] = true
	s.handle(context.Background(), types.EventResponseRequest{ResponseRequest: types.ResponseRequest{Notice: "後から届いた結果", NewGeneration: true}})
	noCommand(t, s)
	if s.cfg.Generation.Current() != current {
		t.Fatal("読み上げ中に新しい世代を始めて、読み上げを止めました")
	}
	// 読み上げが終わったら送る。
	s.sent(context.Background(), types.AudioFrame{ItemID: "playing", GenerationID: current, End: true})
	s.maybeRespond(context.Background())
	response, notice := popResponse(t, s)
	if !strings.Contains(notice, "後から届いた結果") || s.cfg.Generation.Current() == current {
		t.Fatal(notice)
	}
	if response["metadata"].(map[string]string)["generation"] == "" {
		t.Fatal(response)
	}
}

// 同じ応答に最終回答があれば、前置き（phase が commentary）の item は読み上げない。
// 前置きと最終回答が同じ文になり、二重に読み上げてしまうため。
func TestCommentaryItemIsNotSpoken(t *testing.T) {
	s := newTestStage(t)
	request(s, "")
	first, _ := popResponse(t, s)
	serverEventForTest(s, map[string]any{"type": "response.created", "response": map[string]any{"id": "r1", "metadata": first["metadata"]}})
	for _, item := range []map[string]any{{"id": "pre", "phase": "commentary"}, {"id": "final", "phase": "final_answer"}} {
		serverEventForTest(s, map[string]any{"type": "response.output_item.added", "response_id": "r1", "item": map[string]any{"id": item["id"], "type": "message", "phase": item["phase"]}})
		serverEventForTest(s, map[string]any{"type": "response.output_text.delta", "response_id": "r1", "item_id": item["id"], "delta": "ちょっと見てみるね。"})
		serverEventForTest(s, map[string]any{"type": "response.output_text.done", "response_id": "r1", "item_id": item["id"], "text": "ちょっと見てみるね。"})
	}
	var spoken []string
	for len(s.response.Downstream) > 0 {
		if e, ok := (<-s.response.Downstream).(types.EventResponseText); ok && e.Text != "" {
			spoken = append(spoken, e.ItemID+":"+e.Text)
		}
	}
	if strings.Join(spoken, ",") != "final:ちょっと見てみるね。" {
		t.Fatal(spoken)
	}
}

// 一次応答のように前置きの item だけが返った応答は、応答の終わりで前置きを読み上げる。
func TestCommentaryOnlyResponseIsSpokenAtDone(t *testing.T) {
	s := newTestStage(t)
	request(s, "一次応答の指示")
	first, _ := popResponse(t, s)
	serverEventForTest(s, map[string]any{"type": "response.created", "response": map[string]any{"id": "r1", "metadata": first["metadata"]}})
	serverEventForTest(s, map[string]any{"type": "response.output_item.added", "response_id": "r1", "item": map[string]any{"id": "pre", "type": "message", "phase": "commentary"}})
	serverEventForTest(s, map[string]any{"type": "response.output_text.delta", "response_id": "r1", "item_id": "pre", "delta": "ちょっと見てみるね。"})
	serverEventForTest(s, map[string]any{"type": "response.output_text.done", "response_id": "r1", "item_id": "pre", "text": "ちょっと見てみるね。"})
	for len(s.response.Downstream) > 0 {
		if e, ok := (<-s.response.Downstream).(types.EventResponseText); ok {
			t.Fatal("応答の終わりより前に前置きを流しました", e)
		}
	}
	serverEventForTest(s, map[string]any{"type": "response.done", "response": map[string]any{"id": "r1", "status": "completed"}})
	var spoken []string
	done := false
	for len(s.response.Downstream) > 0 {
		if e, ok := (<-s.response.Downstream).(types.EventResponseText); ok {
			if e.Text != "" {
				spoken = append(spoken, e.ItemID+":"+e.Text)
			}
			done = done || e.Done
		}
	}
	if strings.Join(spoken, ",") != "pre:ちょっと見てみるね。" || !done {
		t.Fatal(spoken, done)
	}
}

// セッションは、30分以上たって5分会話がないとき、または55分たったときに張り直す。
func TestShouldRotate(t *testing.T) {
	now := time.Now()
	s := &stage{cfg: Config{RotateAfter: 55 * time.Minute}}
	for _, c := range []struct {
		age, quiet time.Duration
		want       bool
	}{
		{20 * time.Minute, 10 * time.Minute, false},
		{35 * time.Minute, 2 * time.Minute, false},
		{35 * time.Minute, 6 * time.Minute, true},
		{56 * time.Minute, 0, true},
	} {
		s.started, s.lastTalk = now.Add(-c.age), now.Add(-c.quiet)
		if got := s.shouldRotate(now); got != c.want {
			t.Fatal(c, got)
		}
	}
}
