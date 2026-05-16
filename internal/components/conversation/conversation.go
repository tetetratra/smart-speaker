package conversation

import (
	"context"
	"time"

	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

type Config struct {
	LogPath        string
	CalendarClient calendarEventLister
	DiaryReader    DiaryReader
}

type Speaker string

const (
	SpeakerHuman Speaker = "human"
	SpeakerAI    Speaker = "ai"
	SpeakerTool  Speaker = "tool"
)

type UtteranceStatus int

const (
	UtteranceUnplayed UtteranceStatus = iota
	UtterancePlaying
	UtterancePlayed
	UtteranceCanceled
)

type Utterance struct {
	ID              string
	Speaker         Speaker
	StartAt         time.Time
	DurationSeconds float64
	Content         string
	Status          UtteranceStatus
	ResponseID      string
}

type runner struct {
	upstream   chan types.Event
	downstream chan types.Event
	ctx        context.Context
	cancel     context.CancelFunc
	once       bool

	core *conversationCore

	timer  *time.Timer
	timerC <-chan time.Time

	contexts *contextProvider
	logger   *conversationLogger
}

const (
	maxInvalidResponseRetries = 1
	invalidResponseRetryHint  = "前回の出力は契約違反でした。必ず 1 行 1 JSON object の NDJSON 風テキストだけを返してください。各行は {\"type\":\"speech\",\"text\":\"文字列\"} / {\"type\":\"wait\",\"sec\":整数} / {\"type\":\"whiteboard\",\"content\":\"文字列\"} のいずれかにしてください。"
	diaryPromptPrefix         = "以下は過去の会話をまとめた日記です。参考として扱ってください。\n"
	calendarPromptPrefix      = "以下はGoogleカレンダー情報です。会話の参考にしてください。\n\n"
	calendarPromptDays        = 3
	calendarFetchMaxResults   = 30
)

// NewStage は会話タイミング管理のステージを作成します。
func NewStage(cfg Config) *graph.Stage {
	r := &runner{
		upstream:   make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream: make(chan types.Event, graph.DefaultChannelBufferSize),
		contexts:   newContextProvider(cfg.CalendarClient, cfg.DiaryReader),
		logger:     newConversationLogger(cfg.LogPath),
		core:       newConversationCore(),
	}
	return &graph.Stage{
		Upstream:   r.upstream,
		Downstream: r.downstream,
		Run:        r.run,
		CloseFn:    r.close,
	}
}
