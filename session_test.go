package gocomet

import (
	"runtime"
	"testing"
)

func TestSessionLifeCycle(t *testing.T) {
	sc := newSessionContainer()
	var isRunning bool = false
	id := sc.new(func(stopSignal chan bool) {
		isRunning = true
		<-stopSignal
		isRunning = false
	})
	assert(sc.exists(id), t, "failed to create session")
	assert(!isRunning, t, "session should be by default inactive")
	sc.start(id)
	runtime.Gosched()
	assert(isRunning, t, "failed to start session")
	sc.stop(id)
	assert(!isRunning, t, "failed to stop session")
	sc.start(id)
	assert(isRunning, t, "failed to start session again")
	sc.shutdown()
	// time.Sleep(100 * time.Millisecond)
	runtime.Gosched()
	assert(!isRunning, t, "sessions should be closed when the container is shutdown")
}

func simple(stopSignal chan bool) {
	<-stopSignal
}

func assert(ok bool, t *testing.T, msg string) {
	if !ok {
		t.Error(msg)
	}
}

func TestUniqueSessionID(t *testing.T) {
	sc := newSessionContainer()
	defer sc.shutdown()
	id1 := sc.new(simple)
	id2 := sc.new(simple)
	assert(id2 != id1, t, "session IDs must be unique")
}
