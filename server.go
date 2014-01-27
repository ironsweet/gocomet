package gocomet

import (
	"github.com/serverhorror/uuid"
	"log"
	"strings"
	"sync"
)

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

func (c *Server) handshake() (clientId string, err error) {
	clientId, err = c.names.get()
	c.Lock()
	defer c.Unlock()

	routerOutput := c.broker.register(clientId)
	c.sessions[clientId] = newSession(clientId, routerOutput, func() {
		c.Lock()
		defer c.Unlock()
		delete(c.sessions, clientId)
	})
	return
}

/*
Connect may supercede other non-connect waiting channels.
*/
func (c *Server) connect(clientId string) (ch chan *Message, ok bool) {
	if ok = c.names.touch(clientId); !ok {
		return
	}
	c.RLock()
	defer c.RUnlock()

	var ss *Session
	if ss, ok = c.sessions[clientId]; ok {
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
	if strings.Contains(subscription, ",") {
		panic("not supported yet")
	}
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
	log.Printf("[%8.8v]Publish '%v' at '%v'", clientId, data, channel)
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
		ss.channelTimeout <- msg
	}
}
