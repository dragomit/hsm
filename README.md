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
Typically, extended state will be a pointer to a struct, although you are free to use other types.
The state machine is parametrized by this type. If you don't need any extended state,
use `struct{}`:
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
state machine will land in a leaf state.

Similarly, any composite state that's a target of a state transition must have exactly one
of its sub-states marked as initial.

Note that at any given time, state machine must be in exactly one _leaf state_.

## Event Delivery and State Transitions

When an event is delivered to the state machine,
the event is matched against the configured transition,
looking for a matching transition. A matching transition is one where:
 * transition `eventId` matches the `Id` of the delivered event, and
 * transition source state matches the current state or one of its parent states, and
 * transition has no guard condition, or guard conditions is defined and evaluates to true

If more than one transition matches the above conditions, the ambiguity is resolved as follows:
 * transitions defined for a sub-state take precedence over transitions defined for its parent state
 * within a given state, transitions are considered in the order in which they were defined in the code
 * the search ends on the first matching transition

If no matching transition is found, the event is silently ignored.


States may define entry and exit actions, which will be executed each time when the state is entered or exited.





