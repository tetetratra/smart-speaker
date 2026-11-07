package realtime

import (
	"context"
	"fmt"
	"github.com/tetetratra/smart-speaker/internal/types"
	"time"
)

// wakeOnlyInstructionsは、呼びかけだけの発話への応答にだけ付ける指示。
// 呼びかけの言葉は設定で変わるため、システムプロンプトではなく応答ごとに渡す。
const wakeOnlyInstructions = `

ユーザーは呼びかけの言葉だけを言いました。起きていて話を聞けることを、普段の口調で一言だけ伝えてください（例:「起きたよ」「起きてるよ」）。ツールは使わないでください。`

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
	s.wakeOnlyReply = p.WakeOnly
	s.handle(ctx, types.Event{Kind: types.EventLLMRequest, Payload: types.LLMRequest{RequestID: fmt.Sprintf("local-wake/%s", p.PeerID), Text: p.Text}})
}
