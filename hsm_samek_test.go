package hsm

// This file implements the example state machine described in Miro Samek's book
// "Practical Statecharts in C/C++" on page 95.
// See https://www.state-machine.com/doc/PSiCC.pdf

import (
	"bytes"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

const (
	evA = iota
	evB
	evC
	evD
	evE
	evF
	evG
	evH
	evInit
)

type hs struct {
	foo bool
}

func (h *hs) setFoo(e Event) {
	h.foo = true
}

func (h *hs) unsetFoo(e Event) {
	h.foo = false
}

func (h *hs) isFoo(e Event) bool {
	return h.foo
}

func (h *hs) isNotFoo(e Event) bool {
	return !h.foo
}

func TestHsm(t *testing.T) {

	var buf bytes.Buffer

	makeA := func(txt string) func(Event, *hs) {
		return func(Event, *hs) {
			buf.WriteString(txt)
			buf.WriteByte('\n')
		}
	}

	h := hs{}
	sm := StateMachine[*hs]{LocalDefault: true}

	s0 := sm.State("s0").Entry("enter s0", makeA("enter s0")).Exit("exit s0", makeA("exit S0")).Initial().Build()

	s1 := s0.State("s1").Initial().Entry("enter s1", makeA("enter s1")).Exit("exit s1", makeA("exit s1")).Build()

	s11 := s1.State("s11").Initial().Entry("enter s11", makeA("enter s11")).Exit("exit s11", makeA("exit s11")).Build()
	s2 := s0.State("s2").Entry("enter s2", makeA("enter s2")).Exit("exit s2", makeA("exit s2")).Build()
	s21 := s2.State("s21").Initial().Entry("enter s21", makeA("enter s21")).Exit("exit s21", makeA("exit s21")).Build()
	s211 := s21.State("s211").Initial().Entry("enter s211", makeA("enter s211")).Exit("exit s211", makeA("exit s211")).Build()

	s0.AddTransition(evE, s211)

	s1.AddTransition(evD, s0)
	s1.AddTransition(evA, s1)
	s1.AddTransition(evC, s2)

	s11.Transition(evH, s11).Internal().Guard("is foo", func(event Event, h *hs) bool { return h.isFoo(event) }).Build()
	s11.AddTransition(evG, s211)

	s2.AddTransition(evC, s1)
	s2.AddTransition(evF, s11)

	s21.Transition(evH, s21).
		Guard("not foo", func(event Event, h *hs) bool { return h.isNotFoo(event) }).
		Action("set foo", func(event Event, h *hs) { h.setFoo(event) }).
		Build()

	sm.Finalize()
	fmt.Println(sm.DiagramPUML(func(i int) string {
		return string([]byte{'A' + byte(i)})
	}))

	smi := StateMachineInstance[*hs]{SM: &sm, Ext: &h}
	smi.Initialize(Event{evInit, nil})

	buf.WriteString("event A\n")
	smi.Deliver(Event{evA, nil})

	buf.WriteString("event Ext\n")
	smi.Deliver(Event{evE, nil})

	buf.WriteString("event Ext\n")
	smi.Deliver(Event{evE, nil})

	buf.WriteString("event A\n")
	smi.Deliver(Event{evA, nil})

	buf.WriteString("event H\n")
	smi.Deliver(Event{evH, nil})

	buf.WriteString("event H\n")
	smi.Deliver(Event{evH, nil})

	want := `enter s0
enter s1
enter s11
event A
exit s11
exit s1
enter s1
enter s11
event Ext
exit s11
exit s1
enter s2
enter s21
enter s211
event Ext
exit s211
exit s21
exit s2
enter s2
enter s21
enter s211
event A
event H
exit s211
exit s21
enter s21
enter s211
event H
`
	assert.Equal(t, want, buf.String())

}

type falseBuf struct{}

func (f falseBuf) WriteString(s string) {}
func (f falseBuf) WriteByte(b byte)     {}
func (f falseBuf) Reset()               {}

func BenchmarkHsm(b *testing.B) {

	//var buf bytes.Buffer
	var buf falseBuf

	makeA := func(txt string) func(Event, *hs) {
		return func(Event, *hs) {
			buf.WriteString(txt)
			buf.WriteByte('\n')
		}
	}

	sm := StateMachine[*hs]{LocalDefault: true}

	s0 := sm.State("s0").Entry("enter s0", makeA("enter s0")).Exit("exit s0", makeA("exit S0")).Initial().Build()

	s1 := s0.State("s1").Initial().Entry("enter s1", makeA("enter s1")).Exit("exit s1", makeA("exit s1")).Build()

	s11 := s1.State("s11").Initial().Entry("enter s11", makeA("enter s11")).Exit("exit s11", makeA("exit s11")).Build()
	s2 := s0.State("s2").Entry("enter s2", makeA("enter s2")).Exit("exit s2", makeA("exit s2")).Build()
	s21 := s2.State("s21").Initial().Entry("enter s21", makeA("enter s21")).Exit("exit s21", makeA("exit s21")).Build()
	s211 := s21.State("s211").Initial().Entry("enter s211", makeA("enter s211")).Exit("exit s211", makeA("exit s211")).Build()

	s0.AddTransition(evE, s211)

	s1.AddTransition(evD, s0)
	s1.AddTransition(evA, s1)
	s1.AddTransition(evC, s2)

	s11.Transition(evH, s11).Internal().Guard("is foo", func(event Event, h *hs) bool { return h.isFoo(event) }).Build()
	s11.AddTransition(evG, s211)

	s2.AddTransition(evC, s1)
	s2.AddTransition(evF, s11)

	s21.Transition(evH, s21).
		Guard("not foo", func(event Event, h *hs) bool { return h.isNotFoo(event) }).
		Action("set foo", func(event Event, h *hs) { h.setFoo(event) }).
		Build()

	sm.Finalize()

	for i := 0; i < b.N; i++ {

		buf.Reset()
		h := hs{}

		smi := StateMachineInstance[*hs]{SM: &sm, Ext: &h}
		smi.Initialize(Event{evInit, nil})

		buf.WriteString("event A\n")
		smi.Deliver(Event{evA, nil})

		buf.WriteString("event Ext\n")
		smi.Deliver(Event{evE, nil})

		buf.WriteString("event Ext\n")
		smi.Deliver(Event{evE, nil})

		buf.WriteString("event A\n")
		smi.Deliver(Event{evA, nil})

		buf.WriteString("event H\n")
		smi.Deliver(Event{evH, nil})

		buf.WriteString("event H\n")
		smi.Deliver(Event{evH, nil})

	}
}
