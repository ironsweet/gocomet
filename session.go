package gocomet

import (
	"github.com/serverhorror/uuid"
)

type Session struct {
	isActive   bool
	f          func(chan bool)
	stopSignal chan bool
}

type SessionContainer struct {
	sessionCreate chan func(chan bool)
	sessionQuery  chan string
	sessionStart  chan string
	sessionStop   chan string

	done      chan bool
	sessionId chan string

	stopSignal chan bool
}

func newSessionContainer() (sc *SessionContainer) {
	sc = &SessionContainer{
		sessionCreate: make(chan func(chan bool)),
		sessionQuery:  make(chan string),
		sessionStart:  make(chan string),
		sessionStop:   make(chan string),
		done:          make(chan bool),
		sessionId:     make(chan string),
		stopSignal:    make(chan bool),
	}
	go sc.daemon()
	return sc
}

// Maximum number of retry to avoid UUID conflict
const MAX_RETRY = 100

func (sc *SessionContainer) daemon() {
	var isRunning = true
	var sessions = make(map[string]*Session)
	for isRunning {
		select {
		case <-sc.stopSignal:
			isRunning = false
			for _, ss := range sessions {
				ss.stopSignal <- true
			}
		case f := <-sc.sessionCreate:
			var id string
			var limit = MAX_RETRY
			for limit > 0 {
				id = uuid.UUID4()
				if _, ok := sessions[id]; !ok {
					break
				}
				limit--
			}
			if limit > 0 {
				sessions[id] = &Session{f: f, stopSignal: make(chan bool)}
			} else {
				id = "" // no valid ID can be found
			}
			sc.sessionId <- id
		case id := <-sc.sessionQuery:
			_, ok := sessions[id]
			sc.done <- ok
		case id := <-sc.sessionStart:
			ss := sessions[id]
			go ss.f(ss.stopSignal)
			sc.done <- true
		case id := <-sc.sessionStop:
			sessions[id].stopSignal <- true
			sc.done <- true
		}
	}
}

/*
Create a new session.
*/
func (sc *SessionContainer) new(f func(stopSignal chan bool)) string {
	sc.sessionCreate <- f
	return <-sc.sessionId
}

func (sc *SessionContainer) exists(id string) bool {
	sc.sessionQuery <- id
	return <-sc.done
}

func (sc *SessionContainer) start(id string) {
	sc.sessionStart <- id
	<-sc.done
}

func (sc *SessionContainer) stop(id string) {
	sc.sessionStop <- id
	<-sc.done
}

func (sc *SessionContainer) shutdown() {
	sc.stopSignal <- true
}
