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
	ToolResults    *ToolResultSink
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
	GenerationID    uint64
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

	toolResults *ToolResultSink
}

const (
	maxInvalidResponseRetries = 1
	importantRetryPrefix      = "**[重要]** "
	calendarPromptPrefix      = "以下はGoogleカレンダー情報です。会話の参考にしてください。\n\n"
	calendarPromptDays        = 3
	calendarFetchMaxResults   = 30
)

// NewStage は会話タイミング管理のステージを作成します。
func NewStage(cfg Config) *graph.Stage {
	toolResults := cfg.ToolResults
	if toolResults == nil {
		toolResults = NewToolResultSink()
	}
	r := &runner{
		upstream:    make(chan types.Event, graph.DefaultChannelBufferSize),
		downstream:  make(chan types.Event, graph.DefaultChannelBufferSize),
		contexts:    newContextProvider(cfg.CalendarClient),
		logger:      newConversationLogger(cfg.LogPath),
		core:        newConversationCore(),
		toolResults: toolResults,
	}
	return &graph.Stage{
		Upstream:   r.upstream,
		Downstream: r.downstream,
		Run:        r.run,
		CloseFn:    r.close,
	}
}
