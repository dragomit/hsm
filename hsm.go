package hsm

type State struct {
	name        string
	parent      *State
	children    []*State
	initial     *State // initial child state
	validated   bool
	local       bool // whether to use local transitions by default
	entry, exit func(Event)
	transitions []*transition
}

type StateMachine struct {
	top     State
	current *State
}

type Event struct {
	EventId int
	Data    any
}

type transition struct {
	name     string
	internal bool
	local    bool
	eventId  int
	target   *State
	guard    func(Event) bool
	action   func(Event)
}

func (s *State) isLeaf() bool {
	return len(s.children) == 0
}

type StateOption func(s *State)

func WithEntry(f func(e Event)) StateOption {
	return func(s *State) { s.entry = f }
}

func WithExit(f func(e Event)) StateOption {
	return func(s *State) { s.exit = f }
}

func WithInitial() StateOption {
	return func(s *State) {
		p := s.parent
		if p.initial != nil && p.initial != s {
			panic("parent state " + p.name + " already has initial sub-state " + p.initial.name)
		}
		p.initial = s
	}
}

func WithLocalDefault(b bool) StateOption {
	return func(s *State) { s.local = b }
}

type TransitionOption func(s *State, t *transition)

func WithTName(name string) TransitionOption {
	return func(s *State, t *transition) { t.name = name }
}

func WithGuard(f func(e Event) bool) TransitionOption {
	return func(s *State, t *transition) { t.guard = f }
}

func WithAction(f func(e Event)) TransitionOption {
	return func(s *State, t *transition) { t.action = f }
}

func WithInternal() TransitionOption {
	return func(s *State, t *transition) { t.internal = true }
}

func WithLocal() TransitionOption {
	return func(s *State, t *transition) {
		if parent := getParent(s, t.target); parent == nil {
			panic("No point in specifying local transition " + s.name + " -> " + t.target.name)
		}
		t.local = true
	}
}

func (s *State) AddState(name string, opts ...StateOption) *State {
	ss := State{parent: s, name: name, local: s.local}
	for _, opt := range opts {
		opt(&ss)
	}
	s.children = append(s.children, &ss)
	return &ss
}

func (s *State) AddTransition(eventId int, target *State, opts ...TransitionOption) {
	t := transition{
		eventId: eventId,
		target:  target,
	}
	if s.local || target.local {
		// One of the two states has enabled local transitions.
		// This is applicable to this transition only if one of these states is contained
		// (directly or transitively) in the other one.
		if parent := getParent(s, target); parent != nil {
			// this is a transition where local vs external can make a difference
			// use default from the parent state
			t.local = parent.local
		}
	}
	s.transitions = append(s.transitions, &t)
	for _, opt := range opts {
		opt(s, &t)
	}
}

// validate checks that if state is entered, a unique path exists through initial transitions
// to a leaf state
func (s *State) validate() {
	for !s.isLeaf() && !s.validated {
		if s.initial == nil {
			panic("state " + s.name + " must have initial substate")
		}
		s.validated = true
		s = s.initial
	}
}

func (sm *StateMachine) AddState(name string, opts ...StateOption) *State {
	return sm.top.AddState(name, opts...)
}

func (sm *StateMachine) Initialize() {
	// must be able to enter root state
	sm.top.validate()

	var recurseValidate func(*State)
	recurseValidate = func(s *State) {
		for _, t := range s.transitions {
			// must be able to enter any state that's target of a transition
			t.target.validate()
		}
		for _, s1 := range s.children {
			recurseValidate(s1)
		}
	}
	recurseValidate(&sm.top)

	// drill down to the initial leaf state, running entry actions along the way
	e := Event{EventId: -1, Data: nil} // null event
	for s := &sm.top; s != nil; s = s.initial {
		if s.entry != nil {
			s.entry(e)
		}
		sm.current = s
	}
}

func (sm *StateMachine) getTransition(e Event) (*State, *transition) {
	for src := sm.current; src != nil; src = src.parent {
		for _, t := range src.transitions {
			if t.eventId == e.EventId && (t.guard == nil || t.guard(e)) {
				return src, t
			}
		}
	}
	return nil, nil
}

func (sm *StateMachine) Deliver(e Event) {
	if !sm.top.validated {
		panic("State machine must be initialized before delivering the first event")
	}
	src, t := sm.getTransition(e)
	if t == nil {
		return
	}
	if t.internal {
		if t.action != nil {
			t.action(e)
		}
		return
	}

	// we've got ourselves a non-internal transition
	dst := t.target
	var storage1, storage2 [5]*State // avoid slice allocations for HSMs less than 6 levels deep
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
	for s := sm.current; s != lca; s = s.parent {
		if s.exit != nil {
			s.exit(e)
		}
	}

	// execute the transition action
	if t.action != nil {
		t.action(e)
	}

	// move down from just below LCS to dst, entering states
	for j := j; j >= 0; j-- {
		if dstPath[j].entry != nil {
			dstPath[j].entry(e)
		}
	}
	sm.current = dst

	// we have entered dst; proceed with initial transitions if dst is composite state
	for s := dst.initial; s != nil; s = s.initial {
		sm.current = s
		if s.entry != nil {
			s.entry(e)
		}
	}

}

// getParent returns the one of the two states that's (direct or transitive) superstate of the other,
// or nil otherwise.
func getParent(s1, s2 *State) *State {
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
