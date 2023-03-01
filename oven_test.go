package hsm_test

import (
	"fmt"
	"github.com/dragomit/hsm"
	"testing"
)

func TestOven(t *testing.T) {

	// events
	const (
		evOpen = iota
		evClose
		evToast
		evBake
		evOff
		evTimer
	)

	// Define state machine object which holds the state hierarchy.
	// We don't need extended state in this simple example, so we'll use struct{} as extended state type.
	sm := hsm.StateMachine[struct{}]{}

	// Actions are functions that take hsm.Event and extended state, and return no result
	heatingOn := func(e hsm.Event, s struct{}) { fmt.Println("Heating On") }
	heatingOff := func(e hsm.Event, s struct{}) { fmt.Println("Heating Off") }
	lightOn := func(e hsm.Event, s struct{}) { fmt.Println("Light On") }
	lightOff := func(e hsm.Event, s struct{}) { fmt.Println("Light Off") }

	// Create the states, and assign them entry and exit actions as necessary
	// Also mark any states that are targets of automatic initial transitions.
	doorOpen := sm.State("Door Open").Entry("light_on", lightOn).Exit("light_off", lightOff).Build()
	doorClosed := sm.State("Door Closed").Initial().Build()
	heating := doorClosed.State("Heating").Entry("heating_on", heatingOn).Exit("heating_off", heatingOff).Build()
	toasting := heating.State("Toasting").Build()
	baking := heating.State("Baking").Build()
	off := doorClosed.State("Off").Initial().Build()

	// Create transitions. In this simple example, we are not assigning any actions or guards.
	doorClosed.AddTransition(evOpen, doorOpen)
	// Here we use deep history transition.
	doorOpen.Transition(evClose, doorClosed).History(hsm.HistoryDeep).Build()
	doorClosed.AddTransition(evOff, off)
	doorClosed.AddTransition(evBake, baking)
	doorClosed.AddTransition(evToast, toasting)

	// State machine must be finalized before it can be used.
	sm.Finalize()

	// Print out PlantUML diagram for this state machine.
	evMapper := func(ev int) string {
		return []string{"open", "close", "toast", "bake", "off", "timer"}[ev]
	}
	fmt.Println(sm.DiagramPUML(evMapper))

}
