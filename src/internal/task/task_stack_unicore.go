//go:build scheduler.tasks

package task

import "runtime/interrupt"

// currentTask is the current running task, or nil if currently in the scheduler.
var currentTask *Task

// Current returns the current active task.
func Current() *Task {
	return currentTask
}

// Pause suspends the current task and returns to the scheduler.
// This function may only be called when running on a goroutine stack, not when running on the system stack or in an interrupt.
func Pause() {
	// Check whether the canary (the lowest address of the stack) is still
	// valid. If it is not, a stack overflow has occurred.
	if *currentTask.state.canaryPtr != stackCanary {
		runtimePanic("goroutine stack overflow")
	}
	if interrupt.In() {
		runtimePanic("blocked inside interrupt")
	}
	currentTask.state.pause()
}

// Resume the task until it pauses or completes.
// This may only be called from the scheduler.
func (t *Task) Resume() {
	println("DEBUG: Task.Resume() called for task", t)
	currentTask = t
	println("DEBUG: Task.Resume() calling gcData.swap()")
	t.gcData.swap()
	println("DEBUG: Task.Resume() calling t.state.resume()")
	t.state.resume()
	println("DEBUG: Task.Resume() t.state.resume() returned")
	t.gcData.swap()
	currentTask = nil
	println("DEBUG: Task.Resume() completed")
}
