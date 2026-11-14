package graph

import (
	"reflect"
	"testing"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func typeSet(events ...types.Event) map[reflect.Type]struct{} {
	set := map[reflect.Type]struct{}{}
	for _, e := range events {
		set[reflect.TypeOf(e)] = struct{}{}
	}
	return set
}

func TestMatchingStagesFiltersByEventType(t *testing.T) {
	a := &Stage{Name: "a"}
	b := &Stage{Name: "b"}
	targets := []edgeTarget{
		{stage: a, types: typeSet(types.EventRTCSignal{})},
		{stage: b, types: typeSet(types.EventTimelineItem{})},
	}

	got := matchingStages(targets, reflect.TypeOf(types.EventRTCSignal{}))
	if len(got) != 1 || got[0] != a {
		t.Fatalf("matchingStages(EventRTCSignal) = %#v, want only a", got)
	}

	got = matchingStages(targets, reflect.TypeOf(types.EventTimelineItem{}))
	if len(got) != 1 || got[0] != b {
		t.Fatalf("matchingStages(EventTimelineItem) = %#v, want only b", got)
	}
}

// 中身の型が同じでも、イベントの型が違えば別の行き先になる。
func TestMatchingStagesSeparatesEventsSharingData(t *testing.T) {
	a := &Stage{Name: "a"}
	targets := []edgeTarget{{stage: a, types: typeSet(types.EventAudioSent{})}}
	if got := matchingStages(targets, reflect.TypeOf(types.EventAudioFrame{})); len(got) != 0 {
		t.Fatalf("EventAudioFrame が EventAudioSent の辺に流れました: %#v", got)
	}
}

func TestMatchingStagesAllowsUnfilteredEdge(t *testing.T) {
	stage := &Stage{Name: "all"}
	got := matchingStages([]edgeTarget{{stage: stage}}, reflect.TypeOf(types.EventRTCSignal{}))
	if len(got) != 1 || got[0] != stage {
		t.Fatalf("matchingStages() = %#v, want unfiltered stage", got)
	}
}
