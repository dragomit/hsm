package hsm

type State[estate any] struct {
	name        string
	parent      *State[estate]
	children    []*State[estate]
	initial     *State[estate] // initial child state
	validated   bool
	local       bool // whether to use local transitions by default
	entry, exit func(Event, estate)
	transitions []*transition[estate]
}

type StateMachine[estate any] struct {
	top State[estate]
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
	name     string
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

type StateOption[estate any] func(s *State[estate])

func WithEntry[estate any](f func(Event, estate)) StateOption[estate] {
	return func(s *State[estate]) { s.entry = f }
}

func WithExit[estate any](f func(Event, estate)) StateOption[estate] {
	return func(s *State[estate]) { s.exit = f }
}

func WithInitial[estate any]() StateOption[estate] {
	return func(s *State[estate]) {
		p := s.parent
		if p.initial != nil && p.initial != s {
			panic("parent state " + p.name + " already has initial sub-state " + p.initial.name)
		}
		p.initial = s
	}
}

func WithLocalDefault[estate any](b bool) StateOption[estate] {
	return func(s *State[estate]) { s.local = b }
}

type TransitionOption[estate any] func(s *State[estate], t *transition[estate])

func WithTName[estate any](name string) TransitionOption[estate] {
	return func(s *State[estate], t *transition[estate]) { t.name = name }
}

func WithGuard[estate any](f func(Event, estate) bool) TransitionOption[estate] {
	return func(s *State[estate], t *transition[estate]) { t.guard = f }
}

func WithAction[estate any](f func(Event, estate)) TransitionOption[estate] {
	return func(s *State[estate], t *transition[estate]) { t.action = f }
}

func WithInternal[estate any]() TransitionOption[estate] {
	return func(s *State[estate], t *transition[estate]) { t.internal = true }
}

func WithLocal[estate any]() TransitionOption[estate] {
	return func(s *State[estate], t *transition[estate]) {
		if parent := getParent(s, t.target); parent == nil {
			panic("No point in specifying local transition " + s.name + " -> " + t.target.name)
		}
		t.local = true
	}
}

func (s *State[estate]) AddState(name string, opts ...StateOption[estate]) *State[estate] {
	ss := State[estate]{parent: s, name: name, local: s.local}
	for _, opt := range opts {
		opt(&ss)
	}
	s.children = append(s.children, &ss)
	return &ss
}

func (s *State[estate]) AddTransition(eventId int, target *State[estate], opts ...TransitionOption[estate]) {
	t := transition[estate]{
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
func (s *State[estate]) validate() {
	for !s.isLeaf() && !s.validated {
		if s.initial == nil {
			panic("state " + s.name + " must have initial substate")
		}
		s.validated = true
		s = s.initial
	}
}

func (sm *StateMachine[estate]) AddState(name string, opts ...StateOption[estate]) *State[estate] {
	return sm.top.AddState(name, opts...)
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
