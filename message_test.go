package gocomet

import (
	"testing"
)

func TestClientLifeCycle(t *testing.T) {
	b := newBroker()
	ch := b.register("client")
	b.deregister("client")
	_, ok := <-ch
	assert(!ok, t, "channel should be closed after deregister")
}

func TestMessageBroadcast(t *testing.T) {
	b := newBroker()
	ch := b.register("client")
	var msg *SimpleMessage
	go func() {
		msg = <-ch
	}()
	b.broadcast("/foo/bar", "hello")
	assert(len(ch) == 0, t, "nothing should happens")
	b.subscribe("client", "/foo/bar")
	b.broadcast("/foo/bar", "hello again")
	assert(msg.data == "hello again", t, "failed to receive message")
}

func TestChannelUnsubscribe(t *testing.T) {
	b := newBroker()
	clientId := "client"
	ch := b.register(clientId)
	var msg *SimpleMessage
	go func() {
		msg = <-ch
	}()
	b.subscribe(clientId, "/foo/bar")
	b.broadcast("/foo/bar", "hello")
	assert(msg.data == "hello", t, "failed to receive message")
	b.unsubscribe(clientId, "/foo/bar")
	b.broadcast("/foo/bar", "hello again")
	assert(len(ch) == 0, t, "nothing should happens")
}
