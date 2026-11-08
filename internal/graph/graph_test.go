package graph

import (
	"testing"

	types "github.com/tetetratra/smart-speaker/internal/types"
)

func TestMatchingStagesFiltersByEventKind(t *testing.T) {
	a := &Stage{Name: "a"}
	b := &Stage{Name: "b"}
	targets := []edgeTarget{
		{stage: a, kinds: map[types.EventKind]struct{}{types.EventRTCSignal: {}}},
		{stage: b, kinds: map[types.EventKind]struct{}{types.EventTimelineItem: {}}},
	}

	got := matchingStages(targets, types.EventRTCSignal)
	if len(got) != 1 || got[0] != a {
		t.Fatalf("matchingStages(EventRTCSignal) = %#v, want only a", got)
	}

	got = matchingStages(targets, types.EventTimelineItem)
	if len(got) != 1 || got[0] != b {
		t.Fatalf("matchingStages(EventTimelineItem) = %#v, want only b", got)
	}
}

func TestMatchingStagesAllowsUnfilteredEdge(t *testing.T) {
	stage := &Stage{Name: "all"}
	got := matchingStages([]edgeTarget{{stage: stage}}, types.EventRTCSignal)
	if len(got) != 1 || got[0] != stage {
		t.Fatalf("matchingStages() = %#v, want unfiltered stage", got)
	}
}
