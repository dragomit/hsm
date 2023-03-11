package hsm_test

import (
	"fmt"
	"github.com/dragomit/hsm"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestOven(t *testing.T) {

	// event types, enumerated as integers
	const (
		evOpen = iota
		evClose
		evBake
		evOff
	)

	// extended state will keep track of how many times the oven door was opened
	type eState struct {
		opened int
	}

	// Define state machine object which holds the state hierarchy.
	// State machine is parameterized by the extended state. In this case that's *eState.
	sm := hsm.StateMachine[*eState]{}

	// Actions are functions taking hsm.Event and extended state, and returning nothing.
	heatingOn := func(e hsm.Event, s *eState) { fmt.Println("Heating On") }
	heatingOff := func(e hsm.Event, s *eState) { fmt.Println("Heating Off") }
	lightOn := func(e hsm.Event, s *eState) { s.opened++; fmt.Println("Light On") }
	lightOff := func(e hsm.Event, s *eState) { fmt.Println("Light Off") }
	dying := func(e hsm.Event, s *eState) { fmt.Println("Giving up a ghost") }

	// Transition guards are functions taking hsm.Event and extended state, and returning bool.
	// Transition takes place if guard returns true.
	isBroken := func(e hsm.Event, s *eState) bool { return s.opened == 100 }
	isNotBroken := func(e hsm.Event, s *eState) bool { return !isBroken(e, s) }

	// Define the states, and assign them entry and exit actions as necessary
	// Also mark any states that are targets of automatic initial transitions.
	doorOpen := sm.State("Door Open").Entry("light_on", lightOn).Exit("light_off", lightOff).Build()
	doorClosed := sm.State("Door Closed").Initial().Build()
	baking := doorClosed.State("Baking").Entry("heating_on", heatingOn).Exit("heating_off", heatingOff).Build()
	off := doorClosed.State("Off").Initial().Build()

	// Define the transitions.
	doorClosed.Transition(evOpen, doorOpen).Guard("not broken", isNotBroken).Build()
	// Transition to nil state terminates the state machine.
	doorClosed.Transition(evOpen, nil).Guard("broken", isBroken).Action("dying", dying).Build()

	// When door is closed, we return to whichever state we were previously in.
	// We accomplish that using history transition (either deep or shallow history would work here).
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
	smi := hsm.StateMachineInstance[*eState]{SM: &sm, Ext: &eState{}}

	// Initialize the instance. The initial event is merely what's passed to the entry functions,
	// but is otherwise not delivered to the state machine. To drive this point home, we'll use
	// an invalid event id, and nil event data.
	smi.Initialize(hsm.Event{Id: -1, Data: nil})

	// confirm we transitioned to "off" state
	assert.Equal(t, off, smi.Current())

	smi.Deliver(hsm.Event{Id: evBake}) // prints "Heating On"
	assert.Equal(t, baking, smi.Current())

	smi.Deliver(hsm.Event{Id: evOpen}) // prints "Heating Off", "Light On"
	assert.Equal(t, doorOpen, smi.Current())

	smi.Deliver(hsm.Event{Id: evClose}) // prints "Light Off", "Heating On"
	assert.Equal(t, baking, smi.Current())

	// open and close 99 more times
	for i := 0; i < 99; i++ {
		smi.Deliver(hsm.Event{Id: evOpen})
		smi.Deliver(hsm.Event{Id: evClose})
	}
	assert.Equal(t, 100, smi.Ext.opened)
	assert.Equal(t, baking, smi.Current())

	// next time we open the door it should break, and state machine should terminate
	smi.Deliver(hsm.Event{Id: evOpen}) // prints "Giving up a ghost"
	// note: smi.Current() == nil
	// note: if any further events are delivered to the state machine, they will be ignored
}
