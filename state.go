package hsm

import (
	"fmt"
	"strings"
)

// State is a leaf or composite state in a state machine.
// To create a top-level state in a state machine,
// use [hsm.StateMachine.State] method.
// To create a sub-state of a composite state,
// use [hsm.State.SubState] method.
// State (and its containing StateMachine) are parameterized by E - the extended state type.
// E is usually a pointer to a struct containing various quantitative aspects of the object's state,
// as opposed to the qualitative aspects captured through the state machine's discrete states.
// If you don't need an extended state, use struct{} for E.
type State[E any] struct {
	name        string
	alias       string
	parent      *State[E]
	children    []*State[E]
	initial     *State[E] // initial child state
	validated   bool
	entry, exit func(Event, E)
	transitions []*transition[E]
	sm          *StateMachine[E]
	history     History // types of history transitions into this state
}

// StateBuilder provides Fluent API for building new [State].
type StateBuilder[E any] struct {
	parent  *State[E]
	name    string
	options []stateOption[E]
}

// Entry sets func f as the entry action for the state being built.
func (sb *StateBuilder[E]) Entry(f func(Event, E)) *StateBuilder[E] {
	sb.options = append(sb.options, func(s *State[E]) { s.entry = f })
	return sb
}

// Exit sets func f as the exit action for the state being built.
func (sb *StateBuilder[E]) Exit(f func(Event, E)) *StateBuilder[E] {
	sb.options = append(sb.options, func(s *State[E]) { s.exit = f })
	return sb
}

// Initial marks the state being built as initial sub-state of the parent state.
// In other words, Initial creates an automatic initial transition
// from the parent state into the new state being built.
func (sb *StateBuilder[E]) Initial() *StateBuilder[E] {
	opt := func(s *State[E]) {
		p := s.parent
		if p.initial != nil && p.initial != s {
			panic("parent state " + p.name + " already has initial sub-state " + p.initial.name)
		}
		p.initial = s
	}
	sb.options = append(sb.options, opt)
	return sb
}

// Build builds and returns the new state.
func (sb *StateBuilder[E]) Build() *State[E] {
	ss := State[E]{
		parent: sb.parent,
		name:   sb.name,
		alias:  strings.ReplaceAll(sb.name, " ", "_"),
		sm:     sb.parent.sm,
	}
	for _, opt := range sb.options {
		opt(&ss)
	}
	sb.parent.children = append(sb.parent.children, &ss)
	// find and remove this builder in the list of unused stateBuilders
	sm := sb.parent.sm
	for i, sb1 := range sm.stateBuilders {
		if sb == sb1 {
			sm.stateBuilders = append(sm.stateBuilders[:i], sm.stateBuilders[i+1:]...)
		}
		return &ss
	}
	panic(fmt.Sprintf("State %s builder: invalid attempt to use the same builder twice", sb.name))
}

// Event instance are delivered to state machine,
// causing it to run actions and change states.
// EventId identifies type of the event, while Data is an optional arbitrary type
// containing auxiliary event data.
type Event struct {
	EventId int
	Data    any
}

type transition[E any] struct {
	internal   bool
	local      bool
	eventId    int
	target     *State[E]
	guard      func(Event, E) bool
	guardName  string
	action     func(Event, E)
	actionName string
	history    History
}

func (t *transition[E]) String() string {
	var bld strings.Builder
	if t.guard != nil {
		bld.WriteByte('[')
		bld.WriteString(t.guardName)
		bld.WriteByte(']')
	}
	if t.action != nil {
		bld.WriteString(" / ")
		bld.WriteString(t.actionName)
	}
	return bld.String()
}

func (s *State[E]) IsLeaf() bool {
	return len(s.children) == 0
}

// State creates and returns a builder for building a nested sub-state.
func (s *State[E]) State(name string) *StateBuilder[E] {
	sb := &StateBuilder[E]{parent: s, name: name}
	// add to the list of (yet) unused builders
	s.sm.stateBuilders = append(s.sm.stateBuilders, sb)
	return sb
}

// validate checks that if state is entered, a unique path exists through initial transitions
// to a leaf state
func (s *State[E]) validate() {
	for !s.IsLeaf() && !s.validated {
		if s.initial == nil {
			panic("state " + s.name + " must have initial substate")
		}
		s.validated = true
		s = s.initial
	}
}

// Transition creates and returns a builder for the transition from the current state into a target state.
// The transition is triggered by event with the given id.
// The returned builder can be used to further customize the transition,
// such as providing action, guard condition, and transition type.
// To indicate state machine termination, provide nil for target state.
func (s *State[E]) Transition(eventId int, target *State[E]) *TransitionBuilder[E] {
	if target == nil {
		target = &s.sm.terminal
	}
	t := transition[E]{target: target, eventId: eventId}
	tb := &TransitionBuilder[E]{src: s, t: &t}
	// add to the list of (yet) unused builders
	s.sm.transitionBuilders = append(s.sm.transitionBuilders, tb)
	return tb
}

// AddTransition is a convenience method, equivalent to calling s.Transition(eventId, target).Build().
func (s *State[E]) AddTransition(eventId int, target *State[E]) {
	s.Transition(eventId, target).Build()
}

type stateOption[E any] func(s *State[E])

type transitionOption[E any] func(s *State[E], t *transition[E])

// TransitionBuilder provides Fluent API for building transition from one state to another.
// TransitionBuilder allows specifying
// a guard condition that must be true for the transition to take place,
// an action to take when making the transition,
// and a type of transition (external, internal, local).
type TransitionBuilder[E any] struct {
	src     *State[E]
	t       *transition[E]
	options []transitionOption[E]
}

// Guard specifies the guard condition - a function that must return true
// for the transition to take place.
// Guard name need not be unique, and is only used for state machine diagram generation.
func (tb *TransitionBuilder[E]) Guard(name string, f func(Event, E) bool) *TransitionBuilder[E] {
	tb.options = append(tb.options, func(s *State[E], t *transition[E]) { t.guard, t.guardName = f, name })
	return tb
}

// Action specifies the transition action name and function.
// The transition action is invoked after any applicable state exit functions,
// and before any applicable state entry functions.
// Action name need not be unique, and is only used for state machine diagram generation.
func (tb *TransitionBuilder[E]) Action(name string, f func(Event, E)) *TransitionBuilder[E] {
	tb.options = append(tb.options, func(s *State[E], t *transition[E]) { t.action, t.actionName = f, name })
	return tb
}

// Internal specifies that transition should be treated as an internal transition,
// as opposed to the default external transition.
// This can only be specified for self-transitions - i.e. target state must be the same as the source state,
// or other the method will panic.
// Internal transitions differ from external transitions in that no entry or exit functions
// are invoked.
// Internal transitions specified in composite-states will be inherited by all the sub-states,
// unless explicitly overriden.
func (tb *TransitionBuilder[E]) Internal() *TransitionBuilder[E] {
	tb.options = append(tb.options, func(s *State[E], t *transition[E]) { t.internal = true })
	return tb
}

// Local specifies whether the transition should be treated as local or external,
// overriding the default for the state machine.
// This can only be specified for transitions between composite state and one of its
// (direct or transitive) sub-states,
// because the concept of local transitions does not make sense otherwise.
// Local transitions differ from the external ones in that they do not feature
// exit and re-entry from the parent (composite) state.
func (tb *TransitionBuilder[E]) Local(b bool) *TransitionBuilder[E] {
	opt := func(s *State[E], t *transition[E]) {
		if parent := getParent(s, t.target); parent == nil {
			panic("No point in specifying local transition " + s.name + " -> " + t.target.name)
		}
		t.local = b
	}
	tb.options = append(tb.options, opt)
	return tb
}

// History specifies that transition shall occur into the (shallow or deep) history of the target composite state.
// In case the system has not yet visited the composite state,
// the transition will proceed into the composite state's initial sub-state.
func (tb *TransitionBuilder[E]) History(h History) *TransitionBuilder[E] {
	opt := func(s *State[E], t *transition[E]) {
		t.history = h
	}
	tb.options = append(tb.options, opt)
	return tb
}

// Build completes building the transition
func (tb *TransitionBuilder[E]) Build() {
	if tb.src.sm.local {
		// State machine defaults to local transitions. This is applicable to this transition
		// only if one of these states is contained (directly or transitively) in the other one.
		if parent := getParent(tb.src, tb.t.target); parent != nil {
			// this is a transition where local vs external can make a difference
			// use default from the parent state
			tb.t.local = true
		}
	}
	tb.src.transitions = append(tb.src.transitions, tb.t)
	for _, opt := range tb.options {
		opt(tb.src, tb.t)
	}
	// remove from list of unused builders
	sm := tb.src.sm
	for i, tb1 := range sm.transitionBuilders {
		if tb == tb1 {
			sm.transitionBuilders = append(sm.transitionBuilders[:i], sm.transitionBuilders[i+1:]...)
			return
		}
	}
	panic("Invalid attempt to use the same transition builder twice")
}
