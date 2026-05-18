package conversation

type speechStartRule struct{}

func (speechStartRule) Apply(_ *conversationCore, sig signal) ([]effect, bool) {
	if _, ok := sig.(speechStartSignal); !ok {
		return nil, false
	}
	// 発話検知だけでは中断しない。確定した文字起こし(EventHumanUtterance)が来た時点で
	// humanTextRule 側が中断と次リクエスト生成を担当する。
	return nil, true
}
