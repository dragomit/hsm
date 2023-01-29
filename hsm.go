package hsm

type State[estate any] struct {
	name        string
	parent      *State[estate]
	children    []*State[estate]
	initial     *State[estate] // initial child state
	validated   bool
	entry, exit func(Event, estate)
	transitions []*transition[estate]
	sm          *StateMachine[estate]
}

type StateBuilder[estate any] struct {
	parent  *State[estate]
	name    string
	options []stateOption[estate]
}

func (sb *StateBuilder[estate]) Entry(f func(Event, estate)) *StateBuilder[estate] {
	sb.options = append(sb.options, func(s *State[estate]) { s.entry = f })
	return sb
}

func (sb *StateBuilder[estate]) Exit(f func(Event, estate)) *StateBuilder[estate] {
	sb.options = append(sb.options, func(s *State[estate]) { s.exit = f })
	return sb
}

func (sb *StateBuilder[estate]) Initial() *StateBuilder[estate] {
	opt := func(s *State[estate]) {
		p := s.parent
		if p.initial != nil && p.initial != s {
			panic("parent state " + p.name + " already has initial sub-state " + p.initial.name)
		}
		p.initial = s
	}
	sb.options = append(sb.options, opt)
	return sb
}

func (sb *StateBuilder[estate]) Add() *State[estate] {
	ss := State[estate]{parent: sb.parent, name: sb.name, sm: sb.parent.sm}
	for _, opt := range sb.options {
		opt(&ss)
	}
	sb.parent.children = append(sb.parent.children, &ss)
	return &ss
}

type StateMachine[estate any] struct {
	top   State[estate]
	local bool // default for whether transitions should be local
}

type StateMachineInstance[estate any] struct {
	SM      *StateMachine[estate]
	E       estate
	current *State[estate]
}

type Event struct {
	EventId int
	Data    any
}

type transition[estate any] struct {
	internal bool
	local    bool
	eventId  int
	target   *State[estate]
	guard    func(Event, estate) bool
	action   func(Event, estate)
}

func (s *State[estate]) isLeaf() bool {
	return len(s.children) == 0
}

type stateOption[estate any] func(s *State[estate])

type transitionOption[estate any] func(s *State[estate], t *transition[estate])

type TransitionBuilder[estate any] struct {
	src     *State[estate]
	t       *transition[estate]
	options []transitionOption[estate]
}

func (tb *TransitionBuilder[estate]) Guard(f func(Event, estate) bool) *TransitionBuilder[estate] {
	tb.options = append(tb.options, func(s *State[estate], t *transition[estate]) { t.guard = f })
	return tb
}

func (tb *TransitionBuilder[estate]) Action(f func(Event, estate)) *TransitionBuilder[estate] {
	tb.options = append(tb.options, func(s *State[estate], t *transition[estate]) { t.action = f })
	return tb
}

func (tb *TransitionBuilder[estate]) Internal() *TransitionBuilder[estate] {
	tb.options = append(tb.options, func(s *State[estate], t *transition[estate]) { t.internal = true })
	return tb
}

func (tb *TransitionBuilder[estate]) Local(b bool) *TransitionBuilder[estate] {
	opt := func(s *State[estate], t *transition[estate]) {
		if parent := getParent(s, t.target); parent == nil {
			panic("No point in specifying local transition " + s.name + " -> " + t.target.name)
		}
		t.local = b
	}
	tb.options = append(tb.options, opt)
	return tb
}

func (tb *TransitionBuilder[estate]) Add() {
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

func (s *State[estate]) State(name string) *StateBuilder[estate] {
	return &StateBuilder[estate]{parent: s, name: name}
}

func (s *State[estate]) Transition(eventId int, target *State[estate]) *TransitionBuilder[estate] {
	t := transition[estate]{target: target, eventId: eventId}
	return &TransitionBuilder[estate]{src: s, t: &t}
}

func (s *State[estate]) AddTransition(eventId int, target *State[estate]) {
	s.Transition(eventId, target).Add()
}

// validate checks that if state is entered, a unique path exists through initial transitions
// to a leaf state
func (s *State[estate]) validate() {
	for !s.isLeaf() && !s.validated {
		if s.initial == nil {
			panic("state " + s.name + " must have initial substate")
		}
		s.validated = true
		s = s.initial
	}
}

func (sm *StateMachine[estate]) State(name string) *StateBuilder[estate] {
	sm.top.sm = sm
	return sm.top.State(name)
}

func (sm *StateMachine[estate]) Finalize() {
	// must be able to enter root state
	sm.top.validate()

	var recurseValidate func(*State[estate])
	recurseValidate = func(s *State[estate]) {
		for _, t := range s.transitions {
			// must be able to enter any state that's target of a transition
			t.target.validate()
		}
		for _, s1 := range s.children {
			recurseValidate(s1)
		}
	}
	recurseValidate(&sm.top)
}

func (smi *StateMachineInstance[estate]) Initialize() {

	if !smi.SM.top.validated {
		panic("state machine not finalized")
	}
	// drill down to the initial leaf state, running entry actions along the way
	e := Event{EventId: -1, Data: nil} // null event
	for s := &smi.SM.top; s != nil; s = s.initial {
		if s.entry != nil {
			s.entry(e, smi.E)
		}
		smi.current = s
	}
}

func (smi *StateMachineInstance[estate]) getTransition(e Event) (*State[estate], *transition[estate]) {
	for src := smi.current; src != nil; src = src.parent {
		for _, t := range src.transitions {
			if t.eventId == e.EventId && (t.guard == nil || t.guard(e, smi.E)) {
				return src, t
			}
		}
	}
	return nil, nil
}

func (smi *StateMachineInstance[estate]) Deliver(e Event) {
	if smi.current == nil {
		panic("State machine must be initialized before delivering the first event")
	}
	src, t := smi.getTransition(e)
	if t == nil {
		return
	}
	if t.internal {
		if t.action != nil {
			t.action(e, smi.E)
		}
		return
	}

	// we've got ourselves a non-internal transition
	dst := t.target
	var storage1, storage2 [5]*State[estate] // avoid slice allocations for HSMs less than 6 levels deep
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
			s.exit(e, smi.E)
		}
	}

	// execute the transition action
	if t.action != nil {
		t.action(e, smi.E)
	}

	// move down from just below LCS to dst, entering states
	for j := j; j >= 0; j-- {
		if dstPath[j].entry != nil {
			dstPath[j].entry(e, smi.E)
		}
	}
	smi.current = dst

	// we have entered dst; proceed with initial transitions if dst is composite state
	for s := dst.initial; s != nil; s = s.initial {
		smi.current = s
		if s.entry != nil {
			s.entry(e, smi.E)
		}
	}

}

// getParent returns the one of the two states that's (direct or transitive) superstate of the other,
// or nil otherwise.
func getParent[estate any](s1, s2 *State[estate]) *State[estate] {
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
