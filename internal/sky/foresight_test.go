package sky

import (
	"fmt"
	"testing"
	"time"
)

func TestAPlanIsUsedOnlyForTheTrackItWasMadeFor(t *testing.T) {
	s := New(1)
	first, second := &Plan{Length: 1}, &Plan{Length: 2}
	s.Foresee("first", first)
	s.Foresee("second", second)
	if s.plan() != nil {
		t.Fatal("a plan is used before any track has played")
	}
	s.Step(20, 10, 0.033, time.Time{}, "first")
	if s.plan() != first {
		t.Errorf("the first track has plan %v", s.plan())
	}
	s.Step(20, 10, 0.033, time.Time{}, "third")
	if s.plan() != nil {
		t.Errorf("a track with no plan has plan %v", s.plan())
	}
	s.Step(20, 10, 0.033, time.Time{}, "second")
	if s.plan() != second {
		t.Errorf("the second track has plan %v", s.plan())
	}
}

func TestANewPlanForATrackReplacesItsOldOne(t *testing.T) {
	s := New(1)
	s.Step(20, 10, 0.033, time.Time{}, "a")
	s.Foresee("a", &Plan{Length: 1})
	newer := &Plan{Length: 2}
	s.Foresee("a", newer)
	if s.plan() != newer || len(s.foresight.plans) != 1 {
		t.Errorf("the track has plan %v among %d", s.plan(), len(s.foresight.plans))
	}
}

func TestOnlyTheLatestPlansAreKept(t *testing.T) {
	s := New(1)
	for i := range foresightKept + 3 {
		s.Foresee(fmt.Sprint(i), &Plan{})
	}
	if len(s.foresight.plans) != foresightKept {
		t.Fatalf("holding %d plans, want %d", len(s.foresight.plans), foresightKept)
	}
	s.Step(20, 10, 0.033, time.Time{}, "0")
	if s.plan() != nil {
		t.Error("the oldest plan was kept")
	}
}
