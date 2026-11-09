package realtime

import (
	"encoding/json"

	"github.com/tetetratra/smart-speaker/internal/types"
)

// toolCompletion は次の応答要求に添える結果の識別子。結果本文は会話へ別途渡す。
type toolCompletion struct {
	CallID     string `json:"call_id"`
	Name       string `json:"name"`
	generation types.GenerationID
	silent     bool
}

// 待機中に世代が変わった無音処理の指示を、通常会話へ持ち込まない。
func (s *stage) discardOldSilentToolResponses() {
	kept := s.pendingToolResponses[:0]
	for _, completion := range s.pendingToolResponses {
		if !completion.silent || s.cfg.Generation.IsCurrent(completion.generation) {
			kept = append(kept, completion)
		}
	}
	s.pendingToolResponses = kept
}

func (s *stage) toolResultInstructions() string {
	if len(s.pendingToolResponses) == 0 {
		return s.instructions
	}
	encoded, _ := json.Marshal(s.pendingToolResponses)
	return s.instructions + `

ツールの実行結果が会話に追加されました。次の一覧のcall_idに対応する結果を確認してください。
取り消された依頼、報告不要になった結果、既に説明済みの結果は説明せず、他に返答が不要ならwait_for_userで待機してください。
それ以外は結果の要点を短く説明してください。画面に残す必要があれば、口頭説明より先にホワイトボードを更新し、その成功後に説明してください。
結果にinstructionsがあれば従ってください。失敗を成功として伝えず、失敗した操作や実行済みの操作を自動で繰り返さないでください。
通常の会話指示と、無音処理を指定された場合の指示は引き続き守ってください。
今回返ったツール結果:
` + string(encoded)
}
