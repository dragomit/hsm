package hsm

import (
	"bytes"
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
)

type hs struct {
	sm  StateMachine
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

	makeA := func(txt string) func(Event) {
		return func(e Event) {
			buf.WriteString(txt)
			buf.WriteByte('\n')
		}
	}

	h := hs{}

	s0 := h.sm.AddState("s0", WithEntry(makeA("enter s0")), WithExit(makeA("exit S0")), WithInitial(), WithLocalDefault(true))
	s1 := s0.AddState("s1", WithInitial(), WithEntry(makeA("enter s1")), WithExit(makeA("exit s1")))
	s11 := s1.AddState("s11", WithInitial(), WithEntry(makeA("enter s11")), WithExit(makeA("exit s11")))
	s2 := s0.AddState("s2", WithEntry(makeA("enter s2")), WithExit(makeA("exit s2")))
	s21 := s2.AddState("s21", WithInitial(), WithEntry(makeA("enter s21")), WithExit(makeA("exit s21")))
	s211 := s21.AddState("s211", WithInitial(), WithEntry(makeA("enter s211")), WithExit(makeA("exit s211")))

	s0.AddTransition(evE, s211)

	s1.AddTransition(evD, s0)
	s1.AddTransition(evA, s1)
	s1.AddTransition(evC, s2)

	s11.AddTransition(evH, s11, WithInternal(), WithGuard(h.isFoo), WithAction(h.unsetFoo))
	s11.AddTransition(evG, s211)

	s2.AddTransition(evC, s1)
	s2.AddTransition(evF, s11)

	s21.AddTransition(evH, s21, WithGuard(h.isNotFoo), WithAction(h.setFoo))

	h.sm.Initialize()

	buf.WriteString("event A\n")
	h.sm.Deliver(Event{evA, nil})

	buf.WriteString("event E\n")
	h.sm.Deliver(Event{evE, nil})

	buf.WriteString("event E\n")
	h.sm.Deliver(Event{evE, nil})

	buf.WriteString("event A\n")
	h.sm.Deliver(Event{evA, nil})

	buf.WriteString("event H\n")
	h.sm.Deliver(Event{evH, nil})

	buf.WriteString("event H\n")
	h.sm.Deliver(Event{evH, nil})

	want := `enter s0
enter s1
enter s11
event A
exit s11
exit s1
enter s1
enter s11
event E
exit s11
exit s1
enter s2
enter s21
enter s211
event E
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

func BenchmarkHsm(b *testing.B) {

	var buf bytes.Buffer

	makeA := func(txt string) func(Event) {
		return func(e Event) {
			buf.WriteString(txt)
			buf.WriteByte('\n')
		}
	}

	for i := 0; i < b.N; i++ {

		buf.Reset()

		h := hs{}

		s0 := h.sm.AddState("s0", WithEntry(makeA("enter s0")), WithExit(makeA("exit S0")), WithInitial(), WithLocalDefault(true))
		s1 := s0.AddState("s1", WithInitial(), WithEntry(makeA("enter s1")), WithExit(makeA("exit s1")))
		s11 := s1.AddState("s11", WithInitial(), WithEntry(makeA("enter s11")), WithExit(makeA("exit s11")))
		s2 := s0.AddState("s2", WithEntry(makeA("enter s2")), WithExit(makeA("exit s2")))
		s21 := s2.AddState("s21", WithInitial(), WithEntry(makeA("enter s21")), WithExit(makeA("exit s21")))
		s211 := s21.AddState("s211", WithInitial(), WithEntry(makeA("enter s211")), WithExit(makeA("exit s211")))

		s0.AddTransition(evE, s211)

		s1.AddTransition(evD, s0)
		s1.AddTransition(evA, s1)
		s1.AddTransition(evC, s2)

		s11.AddTransition(evH, s11, WithInternal(), WithGuard(h.isFoo), WithAction(h.unsetFoo))
		s11.AddTransition(evG, s211)

		s2.AddTransition(evC, s1)
		s2.AddTransition(evF, s11)

		s21.AddTransition(evH, s21, WithGuard(h.isNotFoo), WithAction(h.setFoo))

		h.sm.Initialize()

		buf.WriteString("event A\n")
		h.sm.Deliver(Event{evA, nil})

		buf.WriteString("event E\n")
		h.sm.Deliver(Event{evE, nil})

		buf.WriteString("event E\n")
		h.sm.Deliver(Event{evE, nil})

		buf.WriteString("event A\n")
		h.sm.Deliver(Event{evA, nil})

		buf.WriteString("event H\n")
		h.sm.Deliver(Event{evH, nil})

		buf.WriteString("event H\n")
		h.sm.Deliver(Event{evH, nil})
	}
}
