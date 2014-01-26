package gocomet

import (
	"container/list"
	"errors"
	"github.com/serverhorror/uuid"
	"sync"
	"time"
)

// Maximum number of retry to avoid conflict
const MAX_ID_GEN_RETRY = 100

// Maximum number of IDs to kept to avoid conflict
const MAX_ID_KEPT_TIME = 10 * time.Minute

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

// Maximum allowed session idele. After that, the session is
// considered as disconnected.
const MAX_SESSION_IDEL = 10 * time.Minute

// The unsent messages are kept temporarily in a mailbox. But only
// last MAILBOX_SIZE messages are kept.
const MAILBOX_SIZE = 1000

type Session struct {
	channelReq   chan bool
	channelResp  chan chan *Message
	channelFail  chan *Message
	channelClose chan bool
}

var closedChannel chan *Message = func() chan *Message {
	ch := make(chan *Message)
	close(ch)
	return ch
}()

func newSession(input chan *Message, cleanup func()) *Session {
	channelReq := make(chan bool)
	channelResp := make(chan chan *Message)
	channelFail := make(chan *Message)
	channelClose := make(chan bool)

	go func() {
		var isConnected, isConnect bool
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
			case msg, ok := <-input:
				if !ok { // upstream channel is closed
					isRunning = false
					isConnected = false
					close(output)
					output = nil
				} else if output == nil { // no downstream channel
					// log.Printf("Saved message: %v", msg)
					mailbox.PushBack(msg)
					if mailbox.Len() > MAILBOX_SIZE {
						mailbox.Remove(mailbox.Front())
					}
				} else {
					// log.Printf("Received message: %v", msg)
					if msg == nil {
						panic("message should not be nil")
					}
					output <- msg
				}
			case b := <-channelReq:
				if !isConnected {
					// no existing active channel
					isConnected = true
					isConnect = b
					// try re-send the messages by using a large size channel
					output = make(chan *Message, mailbox.Len())
					if mailbox.Len() > 0 {
						for e := mailbox.Front(); e != nil; e = e.Next() {
							if e.Value == nil {
								panic("message should not be nil")
							}
							output <- e.Value.(*Message)
						}
						mailbox.Init()
					}
					channelResp <- output
				} else if !isConnect && b {
					// override existing non-connect active channel
					isConnect = true
					close(output)
					output = make(chan *Message)
					channelResp <- output
				} else {
					// active connect channel already exists
					channelResp <- closedChannel
				}
			case msg := <-channelFail:
				if msg != nil {
					mailbox.PushFront(msg)
				}
				isConnected = false
				close(output)
				output = nil
			case <-channelClose:
				isRunning = false
				isConnected = false
				close(output)
				output = nil
				if mailbox.Len() > 0 {
					ch := make(chan *Message)
					go func() {
						for e := mailbox.Front(); e != nil; e = e.Next() {
							if e.Value == nil {
								panic("message should not be nil")
							}
							ch <- e.Value.(*Message)
						}
					}()
					channelResp <- ch
				} else {
					channelResp <- closedChannel
				}
			case <-time.After(MAX_SESSION_IDEL):
				isRunning = false
				isConnected = false
				close(output)
				output = nil
			}
		}

		go cleanup()
	}()

	return &Session{
		channelReq:   channelReq,
		channelResp:  channelResp,
		channelFail:  channelFail,
		channelClose: channelClose,
	}
}

func (ss *Session) obtainChannel(isConnect bool) chan *Message {
	ss.channelReq <- isConnect
	return <-ss.channelResp
}

func (ss *Session) close() chan *Message {
	ss.channelClose <- true
	return <-ss.channelResp
}

/*
The Bayeux protocol implementation V1.0.

A common scenario is that, the client should handshake the server
first, to determine which version of the protocol is used. It doesn't
have any transport related fields or logic either becuase they should
be already taken care of before it's used.
*/
type Server struct {
	*sync.RWMutex
	names    *UniqueStringPool
	sessions map[string]*Session
	broker   *Broker
}

func newServer() *Server {
	return &Server{
		RWMutex:  &sync.RWMutex{},
		names:    newUniqueStringPool(uuid.UUID4),
		sessions: make(map[string]*Session),
		broker:   newBroker(),
	}
}

func (c *Server) handshake() (string, error) {
	return c.names.get()
}

/*
Connect may supercede other non-connect waiting channels.
*/
func (c *Server) connect(clientId string) (ch chan *Message, ok bool) {
	if ok = c.names.touch(clientId); !ok {
		return
	}
	c.Lock()
	defer c.Unlock()

	var ss *Session
	ss, ok = c.sessions[clientId]
	if ok {
		ch = ss.obtainChannel(true)
	} else {
		routerOutput := c.broker.register(clientId)
		ss, ok = newSession(routerOutput, func() {
			c.Lock()
			defer c.Unlock()
			delete(c.sessions, clientId)
		}), true
		c.sessions[clientId] = ss
		ch = ss.obtainChannel(true)
	}
	return
}

func (c *Server) disconnect(clientId string) (ch chan *Message, ok bool) {
	if ok = c.names.touch(clientId); !ok {
		return
	}
	c.Lock()
	defer c.Unlock()

	var ss *Session
	if ss, ok = c.sessions[clientId]; ok {
		delete(c.sessions, clientId)
		ch = ss.close()
	}
	return
}

func (c *Server) subscribe(clientId, subscription string) (ch chan *Message, ok bool) {
	if ok = c.names.touch(clientId); !ok {
		return
	}
	c.broker.subscribe(clientId, subscription)
	c.RLock()
	defer c.RUnlock()

	var ss *Session
	if ss, ok = c.sessions[clientId]; ok {
		ch = ss.obtainChannel(false)
	}
	return
}

func (c *Server) unsubscribe(clientId, subscription string) (ch chan *Message, ok bool) {
	if ok = c.names.touch(clientId); !ok {
		return
	}
	if ok = c.broker.unsubscribe(clientId, subscription); !ok {
		return
	}
	c.RLock()
	defer c.RUnlock()

	var ss *Session
	if ss, ok = c.sessions[clientId]; ok {
		ch = ss.obtainChannel(false)
	}
	return
}

func (c *Server) publish(clientId, channel, data string) (ch chan *Message, ok bool) {
	if ok = c.names.touch(clientId); !ok {
		return
	}
	c.broker.broadcast(channel, data)
	c.RLock()
	defer c.RUnlock()

	var ss *Session
	if ss, ok = c.sessions[clientId]; ok {
		ch = ss.obtainChannel(false)
	}
	return
}

/*
Publish message without client ID.
*/
func (c *Server) whisper(channel, data string) {
	c.broker.broadcast(channel, data)
}

/*
Channel maybe closed if no message is received in given time interval,
or there is any error sending the message to the client.
*/
func (c *Server) closeAndReturn(clientId string, msg *Message) {
	c.RLock()
	defer c.RUnlock()

	if ss, ok := c.sessions[clientId]; ok {
		ss.channelFail <- msg
	}
}
