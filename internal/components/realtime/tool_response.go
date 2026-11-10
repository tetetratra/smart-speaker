package realtime

import "encoding/json"

// toolCompletion は次の応答要求に添える結果の識別子。結果本文は会話へ別途渡す。
type toolCompletion struct {
	CallID string `json:"call_id"`
	Name   string `json:"name"`
}

func (s *stage) toolResultInstructions() string {
	if len(s.pendingToolResponses) == 0 {
		return s.instructions
	}
	encoded, _ := json.Marshal(s.pendingToolResponses)
	return s.instructions + `

ツールの実行結果が会話に追加されました。次の一覧のcall_idに対応する結果を確認してください。
取り消された依頼、報告不要になった結果、既に説明済みの結果は説明せず、他に返答が不要ならwait_for_userで待機してください。
結果に中身があれば、要点を短く説明してください。画面に残す必要があれば、口頭説明より先にホワイトボードを更新し、その成功後に説明してください。
受け付けただけで本当の結果が後から届くもの（statusがstartedなど）は、受け付けたことだけを短く伝え、同じ呼び出しを繰り返さないでください。
結果にinstructionsがあれば従ってください。失敗を成功として伝えず、失敗した操作や実行済みの操作を自動で繰り返さないでください。
通常の会話指示と、報告不要などの指定は引き続き守ってください。
今回返ったツール結果:
` + string(encoded)
}
