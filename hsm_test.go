package hsm

import (
	"fmt"
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

func makeA(txt string) func(Event) {
	return func(e Event) {
		fmt.Println(txt)
	}
}

func TestHsm(t *testing.T) {
	h := hs{}
	h.sm.State.name = "SM"
	h.sm.State.local = true

	s0 := h.sm.AddState("s0", WithEntry(makeA("enter S0")), WithExit(makeA("exit S0")), WithInitial())
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

	fmt.Println("deliver A")
	h.sm.Deliver(Event{evA, nil})

	fmt.Println("deliver E")
	h.sm.Deliver(Event{evE, nil})

	fmt.Println("deliver E")
	h.sm.Deliver(Event{evE, nil})

	fmt.Println("deliver A")
	h.sm.Deliver(Event{evA, nil})

	fmt.Println("deliver H")
	h.sm.Deliver(Event{evH, nil})

	fmt.Println("deliver H")
	h.sm.Deliver(Event{evH, nil})

}
