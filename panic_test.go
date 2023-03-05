package hsm_test

import (
	"github.com/dragomit/hsm"
	"github.com/stretchr/testify/assert"
	"testing"
)

func setup() (*hsm.StateMachine[struct{}], *hsm.State[struct{}], *hsm.State[struct{}], *hsm.State[struct{}]) {
	sm := hsm.StateMachine[struct{}]{}
	foo := sm.State("foo").Build()
	bar := sm.State("bar").Build()
	fooChild := foo.State("fooChild").Build()
	return &sm, foo, bar, fooChild
}

func TestPanicLocal(t *testing.T) {
	_, foo, bar, _ := setup()
	assert.PanicsWithValue(t,
		"Transition foo -> bar can not be local",
		func() { foo.Transition(0, bar).Local(true).Build() },
	)
}

func TestPanicInternal(t *testing.T) {
	_, foo, bar, _ := setup()
	assert.PanicsWithValue(
		t,
		"Transition foo -> bar can not be internal",
		func() { foo.Transition(0, bar).Internal().Build() },
	)
}

func TestPanicNoInitial(t *testing.T) {
	sm, _, _, _ := setup()
	assert.PanicsWithValue(t, "state machine must have initial sub-state", sm.Finalize)
}

func TestPanicNoInitial2(t *testing.T) {
	sm, _, _, _ := setup()
	baz := sm.State("baz").Initial().Build()
	baz1 := baz.State("baz1").Build()
	_ = baz1
	assert.PanicsWithValue(t, "state baz must have initial sub-state", sm.Finalize)
}

func TestPanicNoInitialForTarget(t *testing.T) {
	sm, foo, bar, _ := setup()
	sm.State("initial").Initial().Build()
	bar.AddTransition(0, foo)
	assert.PanicsWithValue(t, "state foo must have initial sub-state", sm.Finalize)
}

func TestPanicTwoInitialTransitions(t *testing.T) {
	sm, _, _, _ := setup()
	sm.State("one").Initial().Build()
	assert.PanicsWithValue(
		t,
		"sub-states two and one can not both be marked initial",
		func() { sm.State("two").Initial().Build() },
	)

}

func TestPanicForgottenTransitionBuild(t *testing.T) {
	sm, foo, bar, _ := setup()
	foo.Transition(0, bar)
	sm.State("initial").Initial().Build()
	assert.PanicsWithValue(t, "transition builder for event 0, foo --> bar left unused. Forgotten call to Build()?", sm.Finalize)
}

func TestPanicForgottenStateBuild(t *testing.T) {
	sm, _, _, _ := setup()
	sm.State("initial").Initial().Build()
	sm.State("forgotten")
	assert.PanicsWithValue(t, "state forgotten builder left unused. Forgotten call to Build()?", sm.Finalize)
}
