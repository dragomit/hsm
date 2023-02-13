package hsm

import (
	"fmt"
	"strings"
)

type History int

const (
	HistoryNone    History = 0
	HistoryShallow History = 1
	HistoryDeep    History = 2
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
	return &ss
}

// StateMachine encapsulates the structure of the entire state machine,
// along with all its contained states, transitions, actions, guards, etc.
// StateMachine is only about the structure - to create an instance,
// deliver events to it and drive it through transitions,
// create [StateMachineInstance] tied to this StateMachine.
type StateMachine[E any] struct {
	top      State[E]
	terminal State[E]
	local    bool    // default for whether transitions should be local
	history  History // types of history transitions used
}

// StateMachineInstance is an instance of a particular StateMachine.
// StateMachineInstance receives events and goes through state transitions,
// executing actions along the way.
// Each StateMachineInstance should have its own,
// independent extended state,
// whose type is parameterized by E.
type StateMachineInstance[E any] struct {
	SM             *StateMachine[E]
	Ext            E
	current        *State[E]
	historyShallow map[*State[E]]*State[E]
	historyDeep    map[*State[E]]*State[E]
	initialized    bool
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
}

// State creates and returns a builder for building a nested sub-state.
func (s *State[E]) State(name string) *StateBuilder[E] {
	return &StateBuilder[E]{parent: s, name: name}
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
	return &TransitionBuilder[E]{src: s, t: &t}
}

// AddTransition is a convenience method, equivalent to calling s.Transition(eventId, target).Build().
func (s *State[E]) AddTransition(eventId int, target *State[E]) {
	s.Transition(eventId, target).Build()
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

// State starts a builder for a top-level state in a state machine.
// There must be at least one top-level state in a state machine,
// and exactly one of those must be marked as initial state.
func (sm *StateMachine[E]) State(name string) *StateBuilder[E] {
	sm.top.sm = sm
	return sm.top.State(name)
}

// Finalize validates and finalizes the state machine structure.
// Finalize must be called before any state machine instances are initialized,
// and state machine structure must not be modified after this method is called.
func (sm *StateMachine[E]) Finalize() {
	// must be able to enter root state
	sm.top.validate()

	var recurseValidate func(*State[E])
	recurseValidate = func(s *State[E]) {
		for _, t := range s.transitions {
			sm.history |= t.history
			t.target.history |= t.history
			// must be able to enter any state that's target of a transition
			t.target.validate()
		}
		for _, s1 := range s.children {
			recurseValidate(s1)
		}
	}
	recurseValidate(&sm.top)
}

// DiagramPUML builds a PlantUML diagram of a finalized state machine.
// evNameMapper provides mapping of event ids to event names.
func (sm *StateMachine[E]) DiagramPUML(evNameMapper func(int) string) string {
	if !sm.top.validated {
		panic("state machine not finalized")
	}

	var (
		bld, bldTrans strings.Builder
		dump          func(indent int, s *State[E])
	)

	dump = func(indent int, s *State[E]) {
		prefix := strings.Repeat("   ", indent)

		if s.name == s.alias {
			fmt.Fprintf(&bld, "%sstate %s", prefix, s.alias)
		} else {
			fmt.Fprintf(&bld, "%sstate \"%s\" as %s", prefix, s.name, s.alias)
		}
		if !s.IsLeaf() {
			bld.WriteString(" {\n")
			for _, child := range s.children {
				dump(indent+1, child)
			}
			bld.WriteString(prefix)
			bld.WriteString("}")
		}
		bld.WriteString("\n")
		if s.parent.initial == s {
			fmt.Fprintf(&bld, "%s[*] --> %s\n", prefix, s.alias)
		}
		for _, t := range s.transitions {
			var hist string
			if t.history == HistoryShallow {
				hist = "[H]"
			} else if t.history == HistoryDeep {
				hist = "[H*]"
			}
			if t.internal {
				fmt.Fprintf(&bld, "%s%s : %s%s\n", prefix, s.alias, evNameMapper(t.eventId), t)
			} else if t.local {
				fmt.Fprintf(&bld, "%s%s --> %s%s : %s%s\n", prefix, s.alias, t.target.alias, hist, evNameMapper(t.eventId), t)
			} else {
				fmt.Fprintf(&bldTrans, "%s --> %s%s : %s%s\n", s.alias, t.target.alias, hist, evNameMapper(t.eventId), t)
			}
		}
	}

	bld.WriteString("@startuml\n\n")
	sm.terminal.alias = "[*]"
	for _, s := range sm.top.children {
		if s != &sm.terminal {
			dump(0, s)
		}
	}
	bld.WriteString(bldTrans.String())
	bld.WriteString("\n@enduml\n")
	return bld.String()
}

// Initialize initializes this instance.
// Before this method returns, state machine will enter its initial leaf state,
// invoking any relevant entry actions.
// The event e is passed into the entry actions as the initial event,
// but is otherwise not delivered to state machine.
func (smi *StateMachineInstance[E]) Initialize(e Event) {

	if !smi.SM.top.validated {
		panic("state machine not finalized")
	}

	if smi.SM.history&HistoryDeep != 0 {
		smi.historyDeep = make(map[*State[E]]*State[E])
	}
	if smi.SM.history&HistoryShallow != 0 {
		smi.historyShallow = make(map[*State[E]]*State[E])
	}

	// drill down to the initial leaf state, running entry actions along the way
	for s := &smi.SM.top; s != nil; s = s.initial {
		if s.entry != nil {
			s.entry(e, smi.Ext)
		}
		smi.current = s
	}
	smi.initialized = true
}

func (smi *StateMachineInstance[E]) getTransition(e Event) (*State[E], *transition[E]) {
	for src := smi.current; src != nil; src = src.parent {
		for _, t := range src.transitions {
			if t.eventId == e.EventId && (t.guard == nil || t.guard(e, smi.Ext)) {
				return src, t
			}
		}
	}
	return nil, nil
}

// Deliver an event to the state machine. Any applicable transitions and actions
// will be completed before this method returns.
func (smi *StateMachineInstance[E]) Deliver(e Event) {
	if !smi.initialized {
		panic("State machine must be initialized before delivering the first event")
	}
	if smi.current == nil {
		return // all events are ignored in the terminal state
	}
	src, t := smi.getTransition(e)
	if t == nil {
		return
	}
	if t.internal {
		if t.action != nil {
			t.action(e, smi.Ext)
		}
		return
	}

	// we've got ourselves a non-internal transition
	dst := t.target
	var storage1, storage2 [5]*State[E] // avoid slice allocations for HSMs less than 6 levels deep
	// path up the tree, starting at src/dst and ending at top state
	srcPath, dstPath := storage1[:0], storage2[:0]
	for s := src; s != nil; s = s.parent {
		srcPath = append(srcPath, s)
	}
	for s := dst; s != nil; s = s.parent {
		dstPath = append(dstPath, s)
	}

	// find LCA - the lowest common ancestor, by walking srcPath/dstPath backwards
	// The highest LCA can be is top state, since we don't allow using this as either src or dst for transitions
	var i, j int
	for i, j = len(srcPath)-2, len(dstPath)-2; i > 0 && j > 0 && srcPath[i] == dstPath[j]; i, j = i-1, j-1 {
	}
	// i and j now point to one state below LCA on srcPath and dstPath
	lca := srcPath[i+1]

	if t.local {
		// Transition is marked as local, which means that src is contained in dst or vice-versa.
		// For local transitions we don't want to leave and re-enter the super-state, and so we lower
		// our positions on srcPath and dstPath one step further.
		lca = srcPath[i]
		i--
		j--
	}

	// move up from current state to LCA, and exit every state along the way (excluding LCA)
	for s := smi.current; s != lca; s = s.parent {
		if s.exit != nil {
			s.exit(e, smi.Ext)
		}
		if s.parent.history&HistoryShallow != 0 {
			smi.historyShallow[s.parent] = s
		}
		if s.parent.history&HistoryDeep != 0 {
			smi.historyDeep[s.parent] = smi.current
		}
	}

	// execute the transition action
	if t.action != nil {
		t.action(e, smi.Ext)
	}

	if dst == &smi.SM.terminal {
		smi.current = nil // state machine has terminated
		return
	}

	// move down from just below LCS to dst, entering states
	for j := j; j >= 0; j-- {
		if dstPath[j].entry != nil {
			dstPath[j].entry(e, smi.Ext)
		}
	}

	// we have entered dst; proceed with initial or history transitions if dst is composite state
	smi.current = dst
	var s *State[E] = dst.initial

	if t.history == HistoryDeep {
		if s = smi.historyShallow[dst]; s != nil {
			// compute path from deep history up to dst
			dstPath = dstPath[:0]
			for ; s != dst; s = s.parent {
				dstPath = append(dstPath, s)
			}
			// now walk backwards, entering states
			for i := len(dstPath) - 1; i >= 0; i-- {
				if dstPath[i].entry != nil {
					dstPath[i].entry(e, smi.Ext)
				}
			}
			smi.current = dstPath[0]
			return
		}
		// first transition into this state, no history, use initial transition
		s = dst.initial
	} else if t.history == HistoryShallow {
		s = smi.historyShallow[dst]
		if s == nil {
			// first transition into this state, no history, use initial transition
			s = dst.initial
		}
	}
	for ; s != nil; s = s.initial {
		smi.current = s
		if s.entry != nil {
			s.entry(e, smi.Ext)
		}
	}
}

// Current returns current (leaf) state, or nil if state machine has terminated.
// This method should only be invoked between [StateMachineInstance.Deliver] invocations.
func (smi *StateMachineInstance[E]) Current() *State[E] {
	return smi.current
}

// getParent returns the one of the two states that's (direct or transitive) superstate of the other,
// or nil otherwise.
func getParent[E any](s1, s2 *State[E]) *State[E] {
	for s := s1.parent; s != nil; s = s.parent {
		if s == s2 {
			return s
		}
	}
	for s := s2.parent; s != nil; s = s.parent {
		if s == s1 {
			return s
		}
	}
	return nil
}
