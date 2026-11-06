package realtime

import (
	"context"
	"fmt"
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
	s.pendingWake = nil
	s.speaking = false
	s.wantResponse = false
	s.rotate(ctx)
	return true
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
	s.handle(ctx, types.Event{Kind: types.EventLLMRequest, Payload: types.LLMRequest{RequestID: fmt.Sprintf("local-wake/%s", p.PeerID), Text: p.Text}})
}
