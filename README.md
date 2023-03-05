# hsm - Hierarchical State Machines in Go

Hsm is a Go language library for implementing hierarchical state machines.

The library implements a subset of UML state charts, with the following main features:
 * Hierarchical states - child states can be nested inside parent states,
   inheriting their behaviors (transitions), while allowing for specialization.
   This is also known as "behavioral inheritance".
 * State entry and exit actions.
 * External, local, and internal transitions.
 * Transition actions.
 * Shallow and deep history transitions.
 * Transition guard conditions.
 * Type-safe extended state.
 * PlantUML diagram generation.
 * High-performance.

## Quick start

The following example illustrates most of the library features:
 * Heater is turned on/off on entry to / exit from the Baking state.
 * Light is turned on every time the oven is opened, and turned off when it's closed.
 * Closing the door triggers transition to history state, which means we return to
   whichever state (Baking or Off) the oven was previously in.
   (Note that this is an over-simplification. 
   Real world ovens can also be switched on/off while the door is open.)
 * When the door is opened for 101st time, the oven breaks!

![Oven state machine image](./oven.png)

```go
import (
	"fmt"
	"github.com/dragomit/hsm"
	"github.com/stretchr/testify/assert"
	"testing"
)

 // events
 const (
     evOpen = iota
     evClose
     evBake
     evOff
 )

 // extended state will keep track of how many times the oven door was opened
 type state struct {
     opened int
 }

 // Define state machine object which holds the state hierarchy.
 // State machine is parameterized by the extended state type. Use struct{} if you don't need it.
 sm := hsm.StateMachine[*state]{}

 // Actions are functions that take hsm.Event and extended state, and return no result
 heatingOn := func(e hsm.Event, s *state) { fmt.Println("Heating On") }
 heatingOff := func(e hsm.Event, s *state) { fmt.Println("Heating Off") }
 lightOn := func(e hsm.Event, s *state) { s.opened++; fmt.Println("Light On") }
 lightOff := func(e hsm.Event, s *state) { fmt.Println("Light Off") }
 dying := func(e hsm.Event, s *state) { fmt.Println("Giving up a ghost") }

 // Transition guards are functions taking hsm.Event and extended state, and returning bool.
 // Transition takes place if event id matches and the guard returns true.
 isBroken := func(e hsm.Event, s *state) bool { return s.opened == 100 }
 isNotBroken := func(e hsm.Event, s *state) bool { return !isBroken(e, s) }

 // Create the states, and assign them entry and exit actions as necessary.
 // Also mark any states that are targets of automatic initial transitions.
 doorOpen := sm.State("Door Open").Entry("light_on", lightOn).Exit("light_off", lightOff).Build()
 doorClosed := sm.State("Door Closed").Initial().Build()
 baking := doorClosed.State("Baking").Entry("heating_on", heatingOn).Exit("heating_off", heatingOff).Build()
 off := doorClosed.State("Off").Initial().Build()

 // Create transitions.
 doorClosed.Transition(evOpen, doorOpen).Guard("not broken", isNotBroken).Build()
 // Transition to nil state terminates the state machine.
 doorClosed.Transition(evOpen, nil).Guard("broken", isBroken).Action("dying", dying).Build()

 // When the door is closed, we return to whichever state we were previously in.
 doorOpen.Transition(evClose, doorClosed).History(hsm.HistoryShallow).Build()
 baking.AddTransition(evOff, off)
 off.AddTransition(evBake, baking)

 // State machine must be finalized before it can be used.
 sm.Finalize()

 // Print PlantUML diagram for this state machine.
 evMapper := func(ev int) string {
     return []string{"open", "close", "bake", "off"}[ev]
 }
 fmt.Println(sm.DiagramPUML(evMapper))

 // Create an instance of this state machine.
 smi := hsm.StateMachineInstance[*state]{SM: &sm, Ext: &state{}}

 // Initialize the instance. The initial event is merely what's passed to the state entry
 // functions, but is otherwise not delivered to the state machine. 
 // To drive this point home, we'll use an invalid event id.
 smi.Initialize(hsm.Event{Id: -1})
 // smi.Current() == off

 smi.Deliver(hsm.Event{Id: evBake}) // prints "Heating On"
 // smi.Current() == baking

 smi.Deliver(hsm.Event{Id: evOpen}) // prints "Heating Off", "Light On"
 // smi.Current() == doorOpen

 smi.Deliver(hsm.Event{Id: evClose}) // prints "Light Off", "Heating On"
 // smi.Current() == baking

 // open and close 99 more times
 for i := 0; i < 99; i++ {
     smi.Deliver(hsm.Event{Id: evOpen})
     smi.Deliver(hsm.Event{Id: evClose})
 }
 // smi.Ext.opened == 100
 // smi.Current() == baking

 // next time we open the door it should break, and state machine should terminate
 smi.Deliver(hsm.Event{Id: evOpen}) // prints "Giving up a ghost"
 // smi.Current() == nil
 // once terminated, state machine remains terminated, and any further events are ignored
``` 

## Extended state

Extended state deals with any quantitative aspects of the state (as opposed to enumerable states).
The state machine is parametrized by this type.
Typically, extended state will be a pointer to a struct, although you are free to use other types.

```go
sm := hsm.StateMachine[*state]{}
```
If you don't need any extended state, use `struct{}`:
```go
sm := hsm.StateMachine[struct{}]{}
```

## Events

Events are represented by a struct containing event id and arbitrary event data:
```go
type Event struct {
	Id int
	Data    any
}
```
The `Id` represents _event type_. For an event to be handled in a given state,
you must specify a transition rule for that state and `Id` combination.

## States

State machine must have at least one top-level state,
and exactly one top-level state must be marked as the _initial_ state.
When initialized, state machine will automatically transition to this state.

States may form an arbitrarily deep hierarchy, with children states nested within the parent states.
If state machine's initial state is a composite state,
then exactly one of its sub-states must be marked as initial.
This rule continues recursively, ensuring that once fully initialized,
state machine will land in a _leaf state_.

Similarly, any composite state that's the target of a state transition must have exactly one
of its sub-states marked as initial.

Once initialized, 
and after any event is delivered to the state machine and processed to completion,
the state machine will be in a leaf state.

## Event Delivery and State Transitions

When an event is delivered to the state machine,
the configured transitions are searched for a transition matching the event:
 * transition `eventId` matches the `Id` of the delivered event, and
 * transition source state matches the current state or any of its ancestors, and
 * transition has no guard condition, or guard condition is defined and evaluates to true

If more than one transition matches the above conditions, the ambiguity is resolved as follows:
 * each sub-state is searched for a matching transition before its parent state,
   with the search starting at the current, leaf state
 * within a given state, transitions are searched in the order in which they were defined in the code
 * the search ends on the first matching transition

If no matching transition is found, the event is silently ignored.

### Run to Completion and the Order of Actions

Events are delivered to the state machine by invoking the `StateMachineInstance.Deliver()` method.
Hsm follows the run-to-completion paradigm - by the time the `Deliver()` method returns,
all state entry/exit and transition actions have been executed, and state machine is in its new leaf state.

#### External transitions

Each transition has:
 * _source state_ - state in which the transition is defined.
   Note that this is not always the same as the current state,
   since current state can be a child (or more distant descendant) of the source state.
 * _target state_ - this is the target of the state transition.

```go
source.Transition(evId, target).Action(...).Build()
```

The order of actions follows the UML specification:
 * First the states are exited, starting with the current state, 
   and going up to (but excluding) the lowest common ancestor (LCA) of the source and target states.
   State exit actions (if defined) are executed in the order in which the states are exited.
 * Next, transition action (if defined) is executed.
 * Next, states are entered, executing any defined entry actions, and landing in the transition target state.
 * Finally, if the transition target is a composite state,
   the initial transitions are followed, eventually landing in a leaf state. 

For example, in the state machine shown in the [Quick Start](#quick-start) section,
delivering the _open_ event while the state machine is in the Baking state will cause
exiting Baking and Door Closed states and entering Door Open state. 
(In this case LCA is the implicit top-level state that contains the entire state machine.)

On the other hand, delivering the _off_ event to the Baking state will exit Baking and enter Off state.
(In this case, LCA is the Door Closed state.)

This transition semantics is known as _external transitions_.
Hsm also supports internal and local transitions, which are described next.

#### Internal Transitions

Internal transitions don't involve any state changes or state entry/exit actions.
To define an internal transition, set transition target to be the same as source,
and mark the transition as internal:
```go
baking.Transition(evChangeTemp, baking).Internal().Action(...).Build()
```

Without specifying internal transition semantics, the transition would be external.
An external self-transition would result in leaving and re-entering the state.

An internal transition defined for a composite state will also apply to any of its
(direct or transitive) sub-states, 
unless a sub-state overrides the behavior by specifying its own transition rule for the same event.

#### Local transitions

When source and target states stand in parent-child (or child-parent) relationship,
the external transition semantics would involve leaving and re-entering the composite state.
The alternative _local transition_ semantics avoids this leaving/re-entering.
For more details,
see this [Wikipedia entry](https://en.wikipedia.org/wiki/UML_state_machine#Local_versus_external_transitions). 

```go
parentState.Transition(evId, childState).Local(true).Action(...).Build()
```

To change the default behavior for the entire state machine to use local rather than external transitions,
set `LocalDefault` attribute of `StateMachine` to `true`:

```go
sm := StateMachine[*state]{LocalDefault: true}
```

## State Machine Structure vs. Instances

`StateMachine` object captures the state chart structure: states, transitions, actions, and guards.
Application should create a single `StateMachine` object (per each structurally different state chart),
but it can create an arbitrary number of independent `StateMachineInstance` objects,
all tied to the same `StateMachine` object.

```go
sm := hsm.StateMachine[*state]{}
state1 := hsm.State(...).Build()
...
sm.Finalize()

smi1 := hsm.StateMachineInstance[*state]{SM: &sm, Ext: &state{}}
smi2 := hsm.StateMachineInstance[*state]{SM: &sm, Ext: &state{}}
...
```

Typically, `StateMachine` object would be created and finalized during the program initialization,
while the `StateMachineInstance` objects can be created as needed.

This separation between the state machine structure and instances minimizes the overhead of creating new instances.


