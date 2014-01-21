package gocomet

import (
	"runtime"
	"testing"
	"time"
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
	runtime.Gosched()
	assert(!isRunning, t, "sessions should be stopped when the container is shutdown")
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

func TestSessionTimeout(t *testing.T) {
	sc := newSessionContainer()
	defer sc.shutdown()
	id := sc.new(simple)
	assert(sc.exists(id), t, "failed to create session")
	sc.setTimeout(1 * time.Nanosecond) // force session timeout
	runtime.Gosched()
	assert(!sc.exists(id), t, "failed to delete timeout session")
}

func TestInvalidSessionId(t *testing.T) {
	sc := newSessionContainer()
	id := "invalid"
	assert(!sc.exists(id), t, "invalid session ID should not exist")
	assert(sc.start(id) != nil, t, "invalid session start should return error")
	assert(sc.stop(id) != nil, t, "invalid session stop should return error")
}
