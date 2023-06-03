package hsm

import (
	"fmt"
	"strings"
)

type edge[E any] struct {
	src, dst *State[E]
}

// DiagramBuilder allows minor customizations of PlantUML diagram layout before building the diagram.
// To create a builder, use StateMachine.DiagramBuilder().
type DiagramBuilder[E any] struct {
	sm           *StateMachine[E]
	evNameMapper func(int) string
	defaultArrow string
	arrows       map[edge[E]]string
}

// DefaultArrow changes the arrow style used for transitions. The default is "-->".
func (db *DiagramBuilder[E]) DefaultArrow(arrow string) *DiagramBuilder[E] {
	db.defaultArrow = arrow
	return db
}

// Arrow specifies the arrow style used for all transitions from src to dst state.
// See here for available arrow styles: https://crashedmind.github.io/PlantUMLHitchhikersGuide/layout/layout.html
func (db *DiagramBuilder[E]) Arrow(src, dst *State[E], arrow string) *DiagramBuilder[E] {
	db.arrows[edge[E]{src, dst}] = arrow
	return db
}

// Build creates and returns PlantUML diagram as a string.
func (db *DiagramBuilder[E]) Build() string {
	sm := db.sm
	evNameMapper := db.evNameMapper
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

		// combine multiple arrows connecting same src and dst into one
		type edgeH struct {
			src, dst *State[E]
			hist     string
		}
		local, normal := make(map[edgeH][]string), make(map[edgeH][]string)

		for _, t := range s.transitions {
			var hist string
			if t.history == HistoryShallow {
				hist = "[H]"
			} else if t.history == HistoryDeep {
				hist = "[H*]"
			}
			if t.internal {
				fmt.Fprintf(&bld, "%s%s : %s%s\n", prefix, s.alias, evNameMapper(t.eventId), t)
				continue
			}
			var m map[edgeH][]string // maps edgeH to label above edgeH
			if t.local {
				m = local
			} else {
				m = normal
			}
			e := edgeH{src: s, dst: t.target, hist: hist}
			m[e] = append(m[e], evNameMapper(t.eventId)+t.String())
		}

		arrow := func(src, dst *State[E]) string {
			if a, ok := db.arrows[edge[E]{src, dst}]; ok {
				return a
			}
			return db.defaultArrow
		}

		for e, labels := range local {
			fmt.Fprintf(&bld, "%s%s %s %s%s : %s\n", prefix, e.src.alias, arrow(e.src, e.dst), e.dst.alias, e.hist, strings.Join(labels, "\\n"))
		}
		for e, labels := range normal {
			fmt.Fprintf(&bldTrans, "%s %s %s%s : %s\n", e.src.alias, arrow(e.src, e.dst), e.dst.alias, e.hist, strings.Join(labels, "\\n"))
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

// DiagramBuilder creates builder for customizing PlantUML diagram before building it.
// evNameMapper provides mapping of event ids to event names.
func (sm *StateMachine[E]) DiagramBuilder(evNameMapper func(int) string) *DiagramBuilder[E] {
	return &DiagramBuilder[E]{
		sm:           sm,
		evNameMapper: evNameMapper,
		defaultArrow: "-->",
		arrows:       make(map[edge[E]]string),
	}
}

// DiagramPUML builds a PlantUML diagram of a finalized state machine.
// This method is a shorthand for sm.DiagramBuilder(evNameMapper).Build().
func (sm *StateMachine[E]) DiagramPUML(evNameMapper func(int) string) string {
	return sm.DiagramBuilder(evNameMapper).Build()
}
