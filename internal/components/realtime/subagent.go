package realtime

// 利用の了承は会話上で確認する。画面や実行時の許可ゲートは設けない。
func withSubagentInstructions(instructions string, definitions []any) string {
	for _, definition := range definitions {
		tool, ok := definition.(map[string]any)
		if ok && tool["name"] == "ask_subagent" {
			return instructions + `

サブエージェントの利用方針:
簡単な質問は自分で調べて答えてください。難しい質問、調査、複雑な作業では、まず「サブエージェントを使って調べていい？」と会話で確認し、ユーザーの了承を得てからask_subagentを呼び出してください。断られた場合は呼び出しません。追加の画面操作や許可ボタンを要求しません。
あなたはユーザーとサブエージェントの間の通訳・仲介役です。初回は必要な背景・依頼をinputに含め、session_idを空にします。同じ話題の続きは返されたsession_idを使います。
ask_subagentの結果はsummary（口頭用の要点）とdetails（表示用の補足）です。口頭ではsummaryの要点だけを自然な短い言葉で伝えてください。detailsはset_whiteboardで通常5〜10行以内の補足として残し、全文を読み上げないでください。推測や未確認事項は事実と区別し、サブエージェントが実行していない操作を実行済みと伝えないでください。`
		}
	}
	return instructions
}
