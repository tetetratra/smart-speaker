package speechsegment

import (
	"context"
	"fmt"
	"github.com/tetetratra/smart-speaker/internal/graph"
	"github.com/tetetratra/smart-speaker/internal/types"
	"strings"
	"unicode/utf8"
)

// NewStage は読み上げ単位への分割だけを担当する。未完の文字列は応答ごとに分離する。
func NewStage() *graph.Stage {
	st := &graph.Stage{Name: "speechsegment", Upstream: make(chan types.Event), Downstream: make(chan types.Event)}
	st.CloseFn = func() error { return nil }
	st.Run = func(ctx context.Context) {
		defer close(st.Downstream)
		var key, buffer string
		index := 0
		emit := func(e types.Event) bool {
			select {
			case <-ctx.Done():
				return false
			case st.Downstream <- e:
				return true
			}
		}
		for {
			e, ok := st.Receive(ctx)
			if !ok {
				return
			}
			p, ok := e.Payload.(types.ResponseText)
			if !ok {
				continue
			}
			nextKey := p.ResponseID + "/" + p.ItemID
			if key != nextKey {
				key = nextKey
				buffer = ""
				index = 0
			}
			buffer += p.Text
			for {
				part, rest := Split(buffer, p.Done)
				if part == "" {
					break
				}
				buffer = rest
				index++
				item := types.TimelineItem{Kind: types.TimelineKindSpeech, ItemID: p.ItemID, GenerationID: p.GenerationID, SequenceID: fmt.Sprintf("%s/%d", key, index), Text: part}
				if !emit(types.Event{Kind: types.EventTimelineItem, Payload: item}) {
					return
				}
			}
			if p.Done {
				if !emit(types.Event{Kind: types.EventAgentTimelineEnd, Payload: types.AgentTimelineEnd{GenerationID: p.GenerationID, ItemID: p.ItemID}}) {
					return
				}
				buffer = ""
				key = ""
			}
		}
	}
	return st
}

// Split は句点を優先し、長い文章は読点または最大80文字で区切る。
func Split(s string, flush bool) (string, string) {
	s = strings.TrimLeft(s, " \t\r\n")
	n := 0
	comma := 0
	for pos, r := range s {
		n++
		end := pos + utf8.RuneLen(r)
		if strings.ContainsRune("。！？!?\n", r) {
			return strings.TrimSpace(s[:end]), s[end:]
		}
		if r == '、' || r == ',' {
			comma = end
		}
		if n >= 80 {
			if comma > 0 {
				end = comma
			}
			return strings.TrimSpace(s[:end]), s[end:]
		}
	}
	if flush && strings.TrimSpace(s) != "" {
		return strings.TrimSpace(s), ""
	}
	return "", s
}
