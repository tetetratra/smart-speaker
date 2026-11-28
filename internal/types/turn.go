package types

// 会話の流れ（conversationflow）と、振り分け（decision）・頭脳AI（brain）・応答生成（realtime-response）のやりとり。

// DecisionChoice は、ユーザーの発話1回をどう扱うかの判定結果。
type DecisionChoice string

const (
	// DecisionRespond は、Realtime がそのまま返す。
	DecisionRespond DecisionChoice = "respond"
	// DecisionDelegateLight は、頭脳AIの軽いモデルに任せる。
	DecisionDelegateLight DecisionChoice = "delegate_light"
	// DecisionDelegateStrong は、頭脳AIの強いモデルに任せる。
	DecisionDelegateStrong DecisionChoice = "delegate_strong"
	// DecisionWait は、発言の続きを待つ（何もしない）。
	DecisionWait DecisionChoice = "wait"
	// DecisionIgnore は、自分宛てではないので応答しない（何もしない）。
	DecisionIgnore DecisionChoice = "ignore"
)

// BrainModel は、頭脳AIに使わせるモデルの重さ。
type BrainModel string

const (
	BrainModelLight  BrainModel = "light"
	BrainModelStrong BrainModel = "strong"
)

// DecisionRequest は、ユーザーの発話1回の判定依頼。会話の履歴は decision が共有のストアから読む。
type DecisionRequest struct {
	TurnID       string
	GenerationID GenerationID
	// Text は今回の発話の文字起こし。履歴に保存済みの記録と同じ内容。
	Text string
}

// Decision は判定結果。
type Decision struct {
	TurnID       string
	GenerationID GenerationID
	Choice       DecisionChoice
}

// BrainRequest は頭脳AIへの依頼。依頼の中身は、会話の履歴（共有のストア）から頭脳AIが読み取る。
type BrainRequest struct {
	RequestID string
	Model     BrainModel
	// GenerationID は、ユーザーの発話から任せたとき、その発話の世代。後から届いた記録への対応なら0。
	// 0でなければ、世代が進んだ時点で頭脳AIは考えるのをやめる。
	GenerationID GenerationID
}

// ResponseRequest は、realtime-response への応答依頼。system の item に Notice を足してから応答を作らせる。
type ResponseRequest struct {
	GenerationID GenerationID
	// Notice は、system の item に時刻と一緒に入れる文（頭脳AIの話す内容、この応答だけへの指示）。
	Notice string
	// NewGeneration は、応答の前に新しい世代を始めることを表す。ターンの外で話すとき（後から届いた記録への対応）に使う。
	NewGeneration bool
}

type EventDecisionRequest struct{ DecisionRequest }
type EventDecision struct{ Decision }
type EventBrainRequest struct{ BrainRequest }
type EventResponseRequest struct{ ResponseRequest }

func (EventDecisionRequest) isEvent() {}
func (EventDecision) isEvent()        {}
func (EventBrainRequest) isEvent()    {}
func (EventResponseRequest) isEvent() {}

// RealtimeSource は、ユーザーの発話・モデルの応答など、Realtime 連携が中身を作った記録の Source。
const RealtimeSource = "realtime"

// BrainSource は、頭脳AIが出した「話す内容」の記録の Source。
const BrainSource = "brain"

// BrainRequestMetadataKey は、頭脳AIの話す内容の記録に、どの依頼への答えかを入れる Metadata のキー。
const BrainRequestMetadataKey = "brain_request"

// BrainSilentMetadataKey は、頭脳AIが何も話さないと判断したことを表す Metadata のキー。値は true。
const BrainSilentMetadataKey = "brain_silent"
