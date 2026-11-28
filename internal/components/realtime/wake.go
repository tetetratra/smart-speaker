package realtime

import (
	"context"
	"github.com/google/uuid"
	"github.com/tetetratra/smart-speaker/internal/types"
	"time"
)

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
	s.pendingRequests = nil
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
	s.wakeUtterance(ctx, p.Text)
}

// wakeUtterance は、ローカルの呼びかけ判定を通った発話を、Realtime の発話と同じユーザーの記録にする。
// 会話の流れがこの記録で振り分けを頼み、記録係から画面にも出る（チャット欄）。
// Realtime の会話にも入れておき、応答依頼が届いたときにこの発話へ答えられるようにする。
func (s *stage) wakeUtterance(ctx context.Context, text string) {
	id := "text/" + uuid.NewString()
	// ユーザーの割り込みは記録係から戻るのを待たず、旧音声を失効させる。
	previous := s.cfg.Generation.Current()
	change, ok := s.controller.Start(id)
	if ok {
		s.recordInterruption(ctx, previous)
	}
	s.emit(ctx, types.EventGenerationChanged{GenerationChanged: change})
	s.transport.send(map[string]any{"type": "conversation.item.create", "item": map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": text}}}})
	s.saveRecord(ctx, types.ConversationRecord{ID: id, Role: types.RoleUser, Text: text, Source: types.RealtimeSource, CreatedAt: time.Now(), GenerationID: change.GenerationID}, false)
}
