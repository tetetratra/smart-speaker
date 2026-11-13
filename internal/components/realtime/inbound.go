package realtime

import (
	"context"

	"github.com/tetetratra/smart-speaker/internal/types"
)

func (s *stage) queueInbound(prompt types.InboundMessage) {
	s.inbound = append(s.inbound, prompt)
}
func (s *stage) startInbound(ctx context.Context) bool {
	if len(s.inbound) == 0 || s.speaking || s.active != "" || s.wantResponse || s.connecting || len(s.pendingText) > 0 || len(s.repairs) > 0 || s.hasPendingOutput() {
		return false
	}
	// 再生端末がつながっていなくても待たずに実行する。音声は誰にも聞こえないが、それは諦める。
	prompt := s.inbound[0]
	s.inbound = s.inbound[1:]
	s.handle(ctx, types.Event{Payload: types.LLMRequest{Text: prompt.Text, Source: prompt.Source, Title: prompt.Title, Metadata: map[string]any{"context": string(prompt.Context)}, Inbound: true}})
	return true
}
