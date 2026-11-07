package realtime

import (
	"context"
	"github.com/tetetratra/smart-speaker/internal/types"
)

func (s *stage) queueScheduled(prompt types.ScheduledPrompt) {
	for i, old := range s.scheduled {
		if old.ID == prompt.ID {
			s.scheduled[i] = prompt
			return
		}
	}
	s.scheduled = append(s.scheduled, prompt)
}
func (s *stage) startScheduled(ctx context.Context) bool {
	if len(s.scheduled) == 0 || s.speaking || s.active != "" || s.wantResponse || s.connecting || len(s.pendingText) > 0 || len(s.repairs) > 0 || s.pendingTools() || s.hasPendingOutput() {
		return false
	}
	index := -1
	for i, prompt := range s.scheduled {
		// 音声通知は再生端末の接続を待つ。無音の処理は端末がなくても実行する。
		if !prompt.Speak || len(s.peers) > 0 {
			index = i
			break
		}
	}
	if index < 0 {
		return false
	}
	prompt := s.scheduled[index]
	s.scheduled = append(s.scheduled[:index], s.scheduled[index+1:]...)
	// 予約時の文言が指示に残っていても、復唱や再予約をせず今実行させる。
	text := "[アプリの予約ジョブ: " + prompt.Name + "]\n予定時刻になりました。次の指示を今すぐ実行してください。指示に時刻や「〜後に」などの表現が含まれていても、それは予約時の表現なので、新しい予約の登録や予約内容の復唱はしないでください。\n指示: " + prompt.Instruction
	if !prompt.Speak {
		text += "\nこのジョブは音声通知不要です。必要な処理を実行し、ユーザーへの発話や完了の読み上げは行わないでください。"
	}
	s.handle(ctx, types.Event{Payload: types.LLMRequest{Text: text, Source: "automation/" + prompt.ID, Silent: !prompt.Speak}})
	return true
}
