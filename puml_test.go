package hsm_test

import (
	"fmt"
	"github.com/dragomit/hsm"
	"testing"
)

const (
	evNewData = iota
	evEnoughData
	evPause
	evSucceeded
	evFailed
	evResume
	evDeepResume
)

func TestPumlExample1(t *testing.T) {
	sm := hsm.StateMachine[struct{}]{}

	state1 := sm.State("State1").Initial().Build()
	state2 := sm.State("State2").Build()
	state3 := sm.State("State3").Build()

	accEnoughData := state3.State("Accumulate enough data").Initial().Build()
	accEnoughData.AddTransition(evNewData, accEnoughData)

	processData := state3.State("Process data").Build()
	accEnoughData.AddTransition(evEnoughData, processData)

	state3.AddTransition(evPause, state2)
	state2.AddTransition(evSucceeded, state3)
	state2.Transition(evResume, state3).History(hsm.HistoryShallow).Build()
	state2.Transition(evDeepResume, state3).History(hsm.HistoryDeep).Build()

	state1.AddTransition(evSucceeded, state2)
	state3.AddTransition(evFailed, state3)

	sm.Finalize()
	fmt.Println(sm.Diagram(func(i int) string {
		return []string{
			"New data",
			"Enough data",
			"Pause",
			"Succeeded",
			"Failed",
			"Resume",
			"Deep resume",
		}[i]
	}))

}
