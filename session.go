package gocomet

import (
	"container/list"
	"errors"
	"log"
	"sync"
	"time"
)

// Maximum number of retry to avoid conflict
const MAX_ID_GEN_RETRY = 100

// Maximum time to keep IDs from being auto-released
const MAX_ID_KEPT_TIME = 30 * time.Minute

type timeAndValue struct {
	value  interface{}
	expire time.Time
}

type UniqueStringPool struct {
	sync.Locker
	newValue func() string
	values   map[string]*list.Element
	order    *list.List
}

func newUniqueStringPool(f func() string) *UniqueStringPool {
	return &UniqueStringPool{&sync.Mutex{}, f, make(map[string]*list.Element), list.New()}
}

func (pool *UniqueStringPool) get() (value string, err error) {
	pool.Lock()
	defer pool.Unlock()

	var limit = MAX_ID_GEN_RETRY
	for limit > 0 {
		value = pool.newValue()
		if _, ok := pool.values[value]; !ok {
			break
		}
		limit--
	}
	if limit == 0 {
		err = errors.New("Unable to obtain new unique ID. Try again later.")
	}

	now := time.Now()
	pool.values[value] = pool.order.PushBack(&timeAndValue{value, now.Add(MAX_ID_KEPT_TIME)})
	for e := pool.order.Front(); e != nil; e = e.Next() {
		if e.Value.(*timeAndValue).expire.After(now) {
			break
		}
		pool.order.Remove(e)
	}

	return
}

func (pool *UniqueStringPool) touch(value string) (ok bool) {
	pool.Lock()
	defer pool.Unlock()

	var e *list.Element
	if e, ok = pool.values[value]; ok {
		pool.order.Remove(e)
		e = pool.order.PushBack(&timeAndValue{value, time.Now().Add(MAX_ID_KEPT_TIME)})
		pool.values[value] = e
	}
	return
}

// Maximum allowed session idel. After that, the session is
// considered as disconnected.
const MAX_SESSION_IDEL time.Duration = 1 * time.Minute

// The unsent messages are kept temporarily in a mailbox. But only
// last MAILBOX_SIZE messages are kept.
const MAILBOX_SIZE = 1000

type Session struct {
	channelReq     chan bool
	channelResp    chan chan *Message
	channelTimeout chan *Message
	channelClose   chan bool
}

var closedChannel chan *Message = func() chan *Message {
	ch := make(chan *Message)
	close(ch)
	return ch
}()

func newSession(id string, input chan *Message, cleanup func()) *Session {
	channelReq := make(chan bool)
	channelResp := make(chan chan *Message)
	channelTimeout := make(chan *Message)
	channelClose := make(chan bool)

	go func() {
		var mailbox *list.List = list.New()
		var output chan *Message
		var isRunning = true
		for isRunning {
			// Session's major responsibilities are:
			// 1. transimit the message from broker to clients;
			// 2. respond to client's channel request;
			// 3. close downstream channel and push back message; and
			// 4. auto-disconnect those clients that exceed max idel time.
			select {
			case msg := <-input:
				if output == nil { // no downstream channel
					log.Printf("[%8.8v]Saved message: %v", id, msg)
					mailbox.PushBack(msg)
					if mailbox.Len() > MAILBOX_SIZE {
						mailbox.Remove(mailbox.Front())
					}
				} else {
					log.Printf("[%8.8v]Received message: %v", id, msg)
					output <- msg
				}

			case isConnect := <-channelReq:
				if output == nil {
					// no existing active channel
					// try queueing the messages by using a large size channel
					ch := convertMailboxToChannel(mailbox)
					if isConnect {
						output = ch
					} else {
						close(ch)
					}
					channelResp <- ch
				} else {
					// active connect channel already exists
					channelResp <- closedChannel
				}

			case msg := <-channelTimeout:
				if msg != nil {
					mailbox.PushFront(msg)
				}
				close(output)
				output = nil

			case <-channelClose:
				isRunning = false
				if output != nil {
					close(output)
				}
				output = nil
				ch := convertMailboxToChannel(mailbox)
				close(ch)
				channelResp <- ch

			case <-time.After(MAX_SESSION_IDEL):
				isRunning = false
				close(output)
				output = nil
			}
		}

		go cleanup()
	}()

	return &Session{
		channelReq:     channelReq,
		channelResp:    channelResp,
		channelTimeout: channelTimeout,
		channelClose:   channelClose,
	}
}

func convertMailboxToChannel(mailbox *list.List) chan *Message {
	if mailbox.Len() == 0 {
		return make(chan *Message)
	}
	ch := make(chan *Message, mailbox.Len())
	for e := mailbox.Front(); e != nil; e = e.Next() {
		if e.Value == nil {
			panic("message should not be nil")
		}
		ch <- e.Value.(*Message)
	}
	mailbox.Init()
	return ch
}

func (ss *Session) obtainChannel(isConnect bool) chan *Message {
	ss.channelReq <- isConnect
	return <-ss.channelResp
}

func (ss *Session) close() chan *Message {
	ss.channelClose <- true
	return <-ss.channelResp
}
