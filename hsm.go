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

// StateMachine encapsulates the structure of the entire state machine,
// along with all its contained states, transitions, actions, guards, etc.
// StateMachine is only about the structure - to create an instance,
// deliver events to it and drive it through transitions,
// create [StateMachineInstance] tied to this StateMachine.
// Zero value of StateMachine is ready for use.
// Do not copy a non-zero StateMachine.
type StateMachine[E any] struct {
	top                State[E]
	terminal           State[E]
	LocalDefault       bool    // default for whether transitions should be local
	history            History // types of history transitions used
	stateBuilders      []*StateBuilder[E]
	transitionBuilders []*TransitionBuilder[E]
}

// StateMachineInstance is an instance of a particular StateMachine.
// StateMachineInstance receives events and goes through state transitions,
// executing actions along the way.
// Each StateMachineInstance should have its own,
// independent extended state,
// whose type is parameterized by E.
// Before using an instance,
// you must set the SM field to assign the instance to a finalized StateMachine.
// Prior to delivering any events to the instance, you must Initialize() it.
type StateMachineInstance[E any] struct {
	SM             *StateMachine[E]
	Ext            E
	current        *State[E]
	historyShallow map[*State[E]]*State[E]
	historyDeep    map[*State[E]]*State[E]
	initialized    bool
}

// State starts a builder for a top-level state in a state machine.
// There must be at least one top-level state in a state machine,
// and exactly one of those must be marked as initial state.
func (sm *StateMachine[E]) State(name string) *StateBuilder[E] {
	sm.top.sm = sm
	sm.top.name = "machine"
	return sm.top.State(name)
}

// Finalize validates and finalizes the state machine structure.
// Finalize must be called before any state machine instances are initialized,
// and state machine structure must not be modified after this method is called.
func (sm *StateMachine[E]) Finalize() {
	sm.top.name = "machine"
	sm.terminal.name = "terminal state"
	// check for unused stateBuilders - likely a forgotten call to Build() method
	for _, sb := range sm.stateBuilders {
		panic(fmt.Sprintf("state %s builder left unused. Forgotten call to Build()?", sb.name))
	}

	// check for unused transition builders - likely a forgotten call to Build() method
	for _, sb := range sm.transitionBuilders {
		panic(fmt.Sprintf(
			"transition builder for event %d, %s --> %s left unused. Forgotten call to Build()?",
			sb.t.eventId, sb.src.name, sb.t.target.name,
		))
	}

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
		if s.entry != nil {
			fmt.Fprintf(&bld, "%s%s : entry / %s\n", prefix, s.alias, s.entryName)
		}
		if s.exit != nil {
			fmt.Fprintf(&bld, "%s%s : exit / %s\n", prefix, s.alias, s.exitName)
		}

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
			if t.eventId == e.Id && (t.guard == nil || t.guard(e, smi.Ext)) {
				return src, t
			}
		}
	}
	return nil, nil
}

// Deliver an event to the state machine, returning whether the event was handled, and in which state.
// Any applicable transitions and actions will be completed before the method returns.
// This method is not reentrant - do not invoke it from within transition actions,
// state entry/exit functions, or transition guard functions.
// If transition action needs to generate a new event,
// arrange for that event to be delivered to instance only _after_ the current Deliver() method returns.
func (smi *StateMachineInstance[E]) Deliver(e Event) (handled bool, src *State[E]) {
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
	handled = true
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
		if s = smi.historyDeep[dst]; s != nil {
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
	return
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
