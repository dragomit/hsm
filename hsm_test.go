package hsm_test

import (
	"bytes"
	"github.com/dragomit/hsm"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestLocalInternalExternal(t *testing.T) {

	const (
		evA = iota
		evB
		evC
		evD
		evE
		evF
		evG
		evH
		evI
	)

	var buf bytes.Buffer

	// action-maker
	makeA := func(txt string) func(hsm.Event, struct{}) {
		return func(hsm.Event, struct{}) {
			buf.WriteString(txt)
			buf.WriteByte('|')
		}
	}

	sm := hsm.StateMachine[struct{}]{}
	s := sm.State("S").Entry("enter S", makeA("enter S")).Exit("exit S", makeA("exit S")).Initial().Build()
	s1 := s.State("S1").Entry("enter S1", makeA("enter S1")).Exit("exit S1", makeA("exit S1")).Initial().Build()
	s11 := s1.State("S11").Entry("enter S11", makeA("enter S11")).Exit("exit S11", makeA("exit S11")).Initial().Build()
	s12 := s1.State("S12").Entry("enter S12", makeA("enter S12")).Exit("Exit S12", makeA("exit S12")).Build()

	s11.Transition(evA, s).Local(true).Action("A", makeA("action A")).Build()
	s11.Transition(evB, s).Local(false).Action("B", makeA("action B")).Build()

	s1.Transition(evC, s).Local(true).Action("C", makeA("action C")).Build()
	s1.Transition(evD, s).Local(false).Action("D", makeA("action D")).Build()

	s1.Transition(evE, s12).Local(true).Action("E", makeA("action E")).Build()
	s1.Transition(evF, s12).Local(false).Action("F", makeA("action F")).Build()

	s.Transition(evG, nil).Action("G", makeA("action G")).Build()
	s1.Transition(evH, s1).Internal().Action("H-in-S1", makeA("H-in-S1")).Build()
	s12.Transition(evH, s12).Internal().Action("H-in-S12", makeA("H-in-S12")).Build()
	s11.Transition(evI, s11).Action("I", makeA("action I")).Build()

	sm.Finalize()

	tests := []struct {
		name    string
		events  []int
		actions string
		state   *hsm.State[struct{}]
	}{
		{
			name:    "local transitions",
			events:  []int{evA, evE},
			actions: "enter S|enter S1|enter S11|exit S11|exit S1|action A|enter S1|enter S11|exit S11|action E|enter S12|",
			state:   s12,
		},
		{
			name:    "external transitions",
			events:  []int{evB, evF},
			actions: "enter S|enter S1|enter S11|exit S11|exit S1|exit S|action B|enter S|enter S1|enter S11|exit S11|exit S1|action F|enter S1|enter S12|",
			state:   s12,
		},
		{
			name:    "local transitions 2",
			events:  []int{evE, evC},
			actions: "enter S|enter S1|enter S11|exit S11|action E|enter S12|exit S12|exit S1|action C|enter S1|enter S11|",
			state:   s11,
		},
		{
			name:    "external transitions 2",
			events:  []int{evD},
			actions: "enter S|enter S1|enter S11|exit S11|exit S1|exit S|action D|enter S|enter S1|enter S11|",
			state:   s11,
		},
		{
			name:    "termination",
			events:  []int{evG, evA, evB, evC, evD, evE, evF},
			actions: "enter S|enter S1|enter S11|exit S11|exit S1|exit S|action G|",
			state:   nil,
		},
		{
			name:    "internal 1",
			events:  []int{evH},
			actions: "enter S|enter S1|enter S11|H-in-S1|",
			state:   s11,
		},
		{
			name:    "internal 2",
			events:  []int{evE, evH},
			actions: "enter S|enter S1|enter S11|exit S11|action E|enter S12|H-in-S12|",
			state:   s12,
		},
		{
			name:    "external self-transition",
			events:  []int{evI},
			actions: "enter S|enter S1|enter S11|exit S11|action I|enter S11|",
			state:   s11,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			smi := hsm.StateMachineInstance[struct{}]{SM: &sm}
			buf.Reset()
			smi.Initialize(hsm.Event{Id: -1, Data: nil})
			for _, ev := range test.events {
				smi.Deliver(hsm.Event{Id: ev, Data: nil})
			}
			if smi.Current() != test.state {
				wants, got := "nil", "nil"
				if test.state != nil {
					wants = test.state.Name()
				}
				if smi.Current() != nil {
					got = smi.Current().Name()
				}
				t.Errorf("Wants state %s got %s", wants, got)
			}
			assert.Equal(t, buf.String(), test.actions)
		})
	}
}
