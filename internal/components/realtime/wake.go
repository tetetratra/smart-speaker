package realtime

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/types"
	"time"
)

// wakeAckTextは呼びかけだけの発話への返事。LLMを通さずに読み上げる。
const wakeAckText = "はい"

func (s *stage) wakeRequired() bool {
	return s.cfg.WakeMode != nil && s.cfg.WakeMode.Snapshot(time.Now()).Required
}

// 必須・不要の切替時はAPI接続も更新し、古い音声バッファやVAD通知を失効させる。
func (s *stage) refreshWakeMode(ctx context.Context) bool {
	required := s.wakeRequired()
	if required == s.wakeModeRequired {
		return false
	}
	s.wakeModeRequired = required
	s.wakeGated = required
	s.pendingWake = nil
	s.speaking = false
	s.wantResponse = false
	s.rotate(ctx)
	return true
}

// refreshWakeConversationは、呼びかけ後の会話中かどうかを反映し、発話ごとの呼びかけが必要ならtrueを返す。
// 会話中の開始・終了では接続を作り直さず、終了時だけクラウド側の未確定の音声を捨てる。
func (s *stage) refreshWakeConversation() bool {
	gated := s.cfg.WakeMode != nil && s.cfg.WakeMode.Snapshot(time.Now()).Gated()
	if gated == s.wakeGated {
		return gated
	}
	s.wakeGated = gated
	if gated {
		if s.ready && s.transport != nil {
			s.transport.send(map[string]any{"type": "input_audio_buffer.clear"})
		}
		s.pendingAudio = nil
		s.speaking = false
		s.awaitingInputCommit = ""
	}
	return gated
}
func (s *stage) localWake(ctx context.Context, p types.LocalWakeUtterance) {
	if s.cfg.WakeMode == nil || !s.peers[p.PeerID] {
		return
	}
	mode := s.cfg.WakeMode.Snapshot(time.Now())
	if !mode.Required || mode.Revision != p.Revision || p.Text == "" {
		return
	}
	if p.WakeOnly {
		s.acknowledgeWake(ctx, p)
		return
	}
	if !s.ready {
		if len(s.pendingWake) < 4 {
			s.pendingWake = append(s.pendingWake, p)
		}
		s.connect(ctx)
		return
	}
	if s.active != "" {
		s.transport.send(map[string]any{"type": "response.cancel"})
	}
	// 既存の応答は世代変更で再生を止める。キャンセル完了後に次の生成を始める。
	s.speaking = false
	s.handle(ctx, types.Event{Kind: types.EventLLMRequest, Payload: types.LLMRequest{RequestID: fmt.Sprintf("local-wake/%s", p.PeerID), Text: p.Text}})
}

// acknowledgeWakeは呼びかけだけの発話に、LLMを通さず短い返事を読み上げる。
// 返事はRealtimeの会話には送らず、ローカルの会話履歴と画面にだけ残す。
func (s *stage) acknowledgeWake(ctx context.Context, p types.LocalWakeUtterance) {
	if s.active != "" && s.transport != nil {
		s.transport.send(map[string]any{"type": "response.cancel"})
	}
	s.speaking = false
	id := "wake-ack/" + uuid.NewString()
	change, _ := s.controller.Start(id)
	s.emit(ctx, types.Event{Kind: types.EventGenerationChanged, Payload: change})
	s.lastActivity = time.Now()
	s.record(ctx, id+"/user", types.RoleUser, p.Text, change.GenerationID, "local-stt")
	s.emit(ctx, types.Event{Kind: types.EventResponseText, Payload: types.ResponseText{GenerationID: change.GenerationID, ResponseID: id, ItemID: id, Text: wakeAckText}})
	s.emit(ctx, types.Event{Kind: types.EventResponseText, Payload: types.ResponseText{GenerationID: change.GenerationID, ResponseID: id, ItemID: id, Done: true}})
	s.record(ctx, id, types.RoleAgent, wakeAckText, change.GenerationID, "wake-ack")
	if s.transport == nil {
		s.connect(ctx)
	}
}
