package gocomet

import (
	"errors"
	"fmt"
	"github.com/serverhorror/uuid"
	"time"
)

type Session struct {
	isActive   bool
	f          func(chan bool)
	stopSignal chan bool
	lastTime   time.Time
}

type SessionContainer struct {
	sessionCreate chan func(chan bool)
	sessionQuery  chan string
	sessionStart  chan string
	sessionStop   chan string

	actionDone  chan bool
	actionError chan error
	sessionId   chan string

	sessionTimeout chan time.Duration
	stopSignal     chan bool
}

func newSessionContainer() (sc *SessionContainer) {
	sc = &SessionContainer{
		sessionCreate:  make(chan func(chan bool)),
		sessionQuery:   make(chan string),
		sessionStart:   make(chan string),
		sessionStop:    make(chan string),
		actionDone:     make(chan bool),
		actionError:    make(chan error),
		sessionId:      make(chan string),
		sessionTimeout: make(chan time.Duration),
		stopSignal:     make(chan bool),
	}
	go sc.daemon()
	return sc
}

// Maximum number of retry to avoid UUID conflict
const MAX_RETRY = 100

// Default timeout for session. Timeout session is removed instantly.
const DEFAULT_TIMEOUT = 1 * time.Hour

func (sc *SessionContainer) daemon() {
	var isRunning = true
	var sessions = make(map[string]*Session)
	var timeout = DEFAULT_TIMEOUT
	for isRunning {
		select {
		case v := <-sc.sessionTimeout:
			timeout = v
			cleanup(sessions, timeout)
		case <-sc.stopSignal:
			isRunning = false
			for _, ss := range sessions {
				if ss.isActive {
					ss.stopSignal <- true
				}
			}
		case f := <-sc.sessionCreate:
			cleanup(sessions, timeout)
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
			sc.actionDone <- ok
		case id := <-sc.sessionStart:
			ss, ok := sessions[id]
			if !ok {
				sc.actionError <- errors.New(fmt.Sprintf("Invalid session ID: %v", id))
			} else {
				go ss.f(ss.stopSignal)
				ss.isActive = true
				sc.actionError <- nil
			}
		case id := <-sc.sessionStop:
			ss, ok := sessions[id]
			if !ok {
				sc.actionError <- errors.New(fmt.Sprintf("Invalid session ID: %v", id))
			} else {
				ss.isActive = false
				ss.stopSignal <- true
				sc.actionError <- nil
			}
		}
	}
}

// Invoked by daemon only
func cleanup(sessions map[string]*Session, timeout time.Duration) {
	limit := time.Now().Add(-timeout)
	var expired []string
	for id, ss := range sessions {
		if !ss.isActive && ss.lastTime.Before(limit) {
			expired = append(expired, id)
		}
	}
	for _, id := range expired {
		delete(sessions, id)
	}
}

/*
Create a new session. Returns "" if unable to create new session.
*/
func (sc *SessionContainer) new(f func(stopSignal chan bool)) string {
	sc.sessionCreate <- f
	return <-sc.sessionId
}

func (sc *SessionContainer) setTimeout(v time.Duration) *SessionContainer {
	sc.sessionTimeout <- v
	return sc
}

func (sc *SessionContainer) exists(id string) bool {
	sc.sessionQuery <- id
	return <-sc.actionDone
}

func (sc *SessionContainer) start(id string) error {
	sc.sessionStart <- id
	return <-sc.actionError
}

func (sc *SessionContainer) stop(id string) error {
	sc.sessionStop <- id
	return <-sc.actionError
}

func (sc *SessionContainer) shutdown() {
	sc.stopSignal <- true
}
