package conversation

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"time"

	calendarapi "smart-speaker/internal/googlecalendar"
	"smart-speaker/internal/graph"
	types "smart-speaker/internal/types"
)

var (
	markdownLinkPattern = regexp.MustCompile(`\[[^\]]*\]\((https?://[^)]+)\)`)
	bareURLPattern      = regexp.MustCompile(`https?://\S+`)
	citationPattern     = regexp.MustCompile("cite[^]+")
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
	invalidResponseRetryHint  = "前回の出力はJSONとして無効でした。必ずJSONのみを返してください。出力は {\"timeline\":[{\"type\":\"wait\",\"sec\":整数},{\"type\":\"speech\",\"text\":\"文字列\"}],\"whiteboard\":{\"content\":\"文字列\"}} の形式に従ってください。whiteboard は不要なら省略可能です。"
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

func (r *runner) run(parent context.Context) {
	r.ctx, r.cancel = context.WithCancel(parent)
	go r.consume()
}

func (r *runner) consume() {
	defer close(r.downstream)
	for {
		select {
		case <-r.ctx.Done():
			r.stopTimer()
			return
		case evt, ok := <-r.upstream:
			if !ok {
				r.stopTimer()
				return
			}
			r.handleEvent(evt)
		case <-r.timerC:
			r.timerC = nil
			r.timer = nil
			r.applyEffects(r.core.Handle(timerElapsedSignal{}))
		}
	}
}

func (r *runner) handleEvent(evt types.Event) {
	sig, ok := signalFromEvent(evt)
	if !ok {
		return
	}
	r.applyEffects(r.core.Handle(sig))
}

func (r *runner) startTimer(d time.Duration) {
	if d <= 0 {
		r.stopTimer()
		r.applyEffects(r.core.Handle(timerElapsedSignal{}))
		return
	}
	if r.timer == nil {
		r.timer = time.NewTimer(d)
		r.timerC = r.timer.C
		return
	}
	if !r.timer.Stop() {
		select {
		case <-r.timer.C:
		default:
		}
	}
	r.timer.Reset(d)
	r.timerC = r.timer.C
}

func (r *runner) stopTimer() {
	if r.timer == nil {
		return
	}
	if !r.timer.Stop() {
		select {
		case <-r.timer.C:
		default:
		}
	}
	r.timer = nil
	r.timerC = nil
}

func (r *runner) emit(evt types.Event) {
	select {
	case <-r.ctx.Done():
		return
	case r.downstream <- evt:
	}
}

func (r *runner) close() error {
	if r.once {
		return nil
	}
	r.once = true
	if r.cancel != nil {
		r.cancel()
	}
	if err := r.logger.Close(); err != nil {
		log.Printf("conversation: log close error: %v", err)
	}
	close(r.upstream)
	return nil
}

type aiOutput struct {
	Timeline   []aiSegment   `json:"timeline"`
	Whiteboard *aiWhiteboard `json:"whiteboard,omitempty"`
}

type aiWhiteboard struct {
	Content string `json:"content"`
}

type aiSegment struct {
	Type string `json:"type"`
	Sec  *int   `json:"sec,omitempty"`
	Text string `json:"text,omitempty"`
}

func parseAIOutput(raw string) (aiOutput, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return aiOutput{}, false
	}
	var out aiOutput
	dec := json.NewDecoder(strings.NewReader(trimmed))
	if err := dec.Decode(&out); err != nil {
		return aiOutput{}, false
	}
	if len(out.Timeline) == 0 {
		return aiOutput{}, false
	}
	speechCount := 0
	for _, seg := range out.Timeline {
		switch seg.Type {
		case "wait":
			if seg.Sec == nil {
				return aiOutput{}, false
			}
		case "speech":
			if strings.TrimSpace(seg.Text) == "" {
				return aiOutput{}, false
			}
			speechCount++
		default:
			return aiOutput{}, false
		}
	}
	if speechCount == 0 {
		return aiOutput{}, false
	}
	if out.Whiteboard != nil {
		out.Whiteboard.Content = strings.TrimSpace(out.Whiteboard.Content)
		if out.Whiteboard.Content == "" {
			return aiOutput{}, false
		}
	}
	return out, true
}

func postWaitDelay(value float64) time.Duration {
	if value <= 0 {
		return 0
	}
	return time.Duration(value * float64(time.Second))
}

func sanitizeSpeech(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	out := markdownLinkPattern.ReplaceAllString(trimmed, "")
	out = bareURLPattern.ReplaceAllString(out, "")
	out = citationPattern.ReplaceAllString(out, "")
	return strings.TrimSpace(out)
}

func sanitizeWait(value *int) int {
	if value == nil {
		return 0
	}
	if *value < 0 {
		return 0
	}
	if *value > 5 {
		return 5
	}
	return *value
}

func waitDelay(value *int) time.Duration {
	return postWaitDelay(normalizeWaitSeconds(value))
}

func normalizeWaitSeconds(value *int) float64 {
	if value == nil {
		return 0
	}
	sec := sanitizeWait(value)
	return float64(sec)
}

func utteranceSource(_ *Utterance) string {
	return "conversation"
}

func formatCalendarPrompt(events []calendarapi.Event, day0 time.Time) string {
	labels := []string{"今日", "明日", "明後日"}
	grouped := make([][]string, calendarPromptDays)
	dayIndex := make(map[string]int, calendarPromptDays)
	for i := 0; i < calendarPromptDays; i++ {
		d := day0.AddDate(0, 0, i)
		dayIndex[d.Format("2006-01-02")] = i
	}
	for _, event := range events {
		startAt, ok := eventStartTime(event.Start)
		if !ok {
			continue
		}
		idx, ok := dayIndex[startAt.Format("2006-01-02")]
		if !ok {
			continue
		}
		grouped[idx] = append(grouped[idx], formatCalendarEventLine(event))
	}
	var b strings.Builder
	b.WriteString(calendarPromptPrefix)
	for i := 0; i < calendarPromptDays; i++ {
		b.WriteString("[")
		b.WriteString(labels[i])
		b.WriteString("]\n")
		lines := grouped[i]
		if len(lines) == 0 {
			b.WriteString("- 予定なし\n")
		} else {
			for _, line := range lines {
				b.WriteString("- ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		if i < calendarPromptDays-1 {
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func formatCalendarEventLine(event calendarapi.Event) string {
	title := strings.TrimSpace(event.Summary)
	if title == "" {
		title = "(タイトルなし)"
	}
	start := formatCalendarEventClock(event.Start, false)
	end := formatCalendarEventClock(event.End, true)
	if start == "" && end == "" {
		return title
	}
	if end == "" {
		return strings.TrimSpace(start + " " + title)
	}
	return strings.TrimSpace(start + "-" + end + " " + title)
}

func eventStartTime(start calendarapi.EventTime) (time.Time, bool) {
	if start.DateTime != "" {
		t, err := time.Parse(time.RFC3339, start.DateTime)
		if err != nil {
			return time.Time{}, false
		}
		local := t.In(time.Local)
		return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local), true
	}
	if start.Date != "" {
		t, err := time.ParseInLocation("2006-01-02", start.Date, time.Local)
		if err != nil {
			return time.Time{}, false
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local), true
	}
	return time.Time{}, false
}

func formatCalendarEventClock(dt calendarapi.EventTime, isEnd bool) string {
	if dt.Date != "" && dt.DateTime == "" {
		if isEnd {
			return ""
		}
		return "終日"
	}
	if dt.DateTime != "" {
		if t, err := time.Parse(time.RFC3339, dt.DateTime); err == nil {
			return t.In(time.Local).Format("15:04")
		}
	}
	return ""
}
