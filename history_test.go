package hsm_test

import (
	"github.com/dragomit/hsm"
	"testing"
)

const (
	evB = iota
	evAshallow
	evAdeep
	evA1
	evA11
	evA12
)

var evNames []string = []string{"evB", "evAshallow", "evAdeep", "evA1", "evA11", "evA12"}

func TestHistory(t *testing.T) {
	sm := hsm.StateMachine[struct{}]{}
	stA := sm.State("A").Build()
	stA1 := stA.State("A1").Build()
	stA2 := stA.State("A2").Initial().Build()
	stA11 := stA1.State("A11").Build()
	stA12 := stA1.State("A12").Initial().Build()
	stB := sm.State("B").Initial().Build()

	stA.AddTransition(evB, stB)
	stB.Transition(evAshallow, stA).History(hsm.HistoryShallow).Build()
	stB.Transition(evAdeep, stA).History(hsm.HistoryDeep).Build()
	stB.AddTransition(evA1, stA1)
	stB.AddTransition(evA11, stA11)
	stB.AddTransition(evA12, stA12)

	sm.Finalize()

	var tests = []struct {
		name       string
		events     []int
		finalState *hsm.State[struct{}]
	}{
		{
			name:       "initial transition to shall history",
			events:     []int{evAshallow},
			finalState: stA2,
		},
		{
			name:       "initial transition to deep history",
			events:     []int{evAdeep},
			finalState: stA2,
		},
		{
			name:       "shallow history",
			events:     []int{evA11, evB, evAshallow},
			finalState: stA12,
		},
		{
			name:       "shallow history2",
			events:     []int{evAshallow, evB, evAshallow},
			finalState: stA2,
		},
		{
			name:       "deep history",
			events:     []int{evA11, evB, evAdeep},
			finalState: stA11,
		},
	}

	assertState := func(t *testing.T, want, actual *hsm.State[struct{}]) {
		if want != actual {
			t.Errorf("Want %s got %s", want.Name(), actual.Name())
		}
	}

	// fmt.Println(sm.DiagramPUML(func(ev int) string { return evNames[ev] }))
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			smi := hsm.StateMachineInstance[struct{}]{
				SM: &sm,
			}
			smi.Initialize(hsm.Event{EventId: -1, Data: nil})
			assertState(t, stB, smi.Current())
			for _, ev := range test.events {
				smi.Deliver(hsm.Event{EventId: ev, Data: nil})
			}
			assertState(t, test.finalState, smi.Current())
		})
	}

}
