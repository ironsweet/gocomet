package gocomet

import (
	// "log"
	"runtime"
	"testing"
)

func TestHandshake(t *testing.T) {
	// log.Println("Testing handshake...")
	s := newServer()
	c1, err := s.handshake()
	assert(err == nil, t, "simple handshake should not fail")
	c2, err := s.handshake()
	assert(err == nil, t, "simple handshake should not fail")
	assert(c1 != c2, t, "client ID should not conflict")
}

func TestConnect(t *testing.T) {
	// log.Println("Testing connect...")
	s := newServer()
	c1, _ := s.handshake()
	_, ok := s.connect(c1)
	assert(ok, t, "failed to connect simple client")
	_, ok = s.connect("invalid")
	assert(!ok, t, "invalid client should not connect")
}

func TestDisconnect(t *testing.T) {
	// log.Println("Testing disconnect...")
	s := newServer()
	_, ok := s.disconnect("invalid")
	assert(!ok, t, "cannot disconnect an non-exist client")
	c1, _ := s.handshake()
	_, ok = s.disconnect(c1)
	assert(!ok, t, "cannot disconnect an un-connected client")
	ch, _ := s.connect(c1)
	_, ok = s.disconnect(c1)
	assert(ok, t, "failed to disconnect a connected client")
	_, ok = <-ch
	assert(!ok, t, "channel should be closed after disconnect")
}

func TestSubscribe(t *testing.T) {
	// log.Println("Testing subscribe...")
	s := newServer()
	_, ok := s.subscribe("invalid", "/foo/bar")
	assert(!ok, t, "cannot subscribe w/o client ID")
	c1, _ := s.handshake()
	_, ok = s.subscribe(c1, "/foo/bar")
	assert(!ok, t, "cannot subscribe w/o connect first")
	s.connect(c1)
	_, ok = s.subscribe(c1, "/foo/bar")
	assert(ok, t, "failed to subscribe")
}

func TestUnsubscribe(t *testing.T) {
	// log.Println("Testing unsubscribe...")
	s := newServer()
	_, ok := s.unsubscribe("invalid", "/foo/bar")
	assert(!ok, t, "cannot unsubscribe w/o client ID")
	c1, _ := s.handshake()
	_, ok = s.unsubscribe(c1, "/foo/bar")
	assert(!ok, t, "cannot unsubscribe w/o connect first")
	s.connect(c1)
	_, ok = s.unsubscribe(c1, "/foo/bar")
	assert(!ok, t, "cannot unsubscribe w/o subscribe first")
	s.subscribe(c1, "/foo/bar")
	_, ok = s.unsubscribe(c1, "/foo/bar")
	assert(ok, t, "failed to unsubscribe")
}

func TestPublish(t *testing.T) {
	// log.Println("Testing publish...")
	s := newServer()
	_, ok := s.publish("invalid", "/foo/bar", "ping")
	assert(!ok, t, "cannot publish with invalid client ID")

	c1, _ := s.handshake()
	_, ok = s.publish(c1, "/foo/bar", "ping")
	assert(!ok, t, "cannot publish w/o connect first")

	s.connect(c1)
	_, ok = s.publish(c1, "/foo/bar", "ping")
	assert(ok, t, "failed to publish to void")

	c2, _ := s.handshake()
	ch, ok := s.connect(c2)
	s.subscribe(c2, "/foo/bar")
	var msg string
	go func() { msg = (<-ch).data }()
	s.publish(c1, "/foo/bar", "ping")
	runtime.Gosched()
	assert(msg == "ping", t, "failed to receive the delivered message")
}

func TestWhisper(t *testing.T) {
	// log.Println("Testing whisper...")
	s := newServer()
	s.whisper("/foo/bar", "ping")

	c1, _ := s.handshake()
	ch, _ := s.connect(c1)
	var msg string
	go func() { msg = (<-ch).data }()
	s.subscribe(c1, "/foo/bar")
	s.whisper("/foo/bar", "ping")
	runtime.Gosched() // give msg receiver a chance
	assert(msg == "ping", t, "failed to receive whipered message (got %v)", msg)
}

func TestTwoConnectionRestrict(t *testing.T) {
	// log.Println("Testing 2 connections restrict...")
	s := newServer()
	c1, _ := s.handshake()
	ch1, _ := s.connect(c1)
	var msg string
	go func() { msg = (<-ch1).data }()
	ch2, _ := s.subscribe(c1, "/foo/bar")
	_, ok := <-ch2
	assert(!ok, t, "only one active channel is allowed")

	c2, _ := s.handshake()
	s.connect(c2)
	s.publish(c2, "/foo/bar", "ping")
	runtime.Gosched()
	assert(msg == "ping", t, "failed to receive message from previous active connect")

	s.closeAndReturn(c1, nil)
	_, ok = <-ch1
	assert(!ok, t, "active channel should be closed after disconnect")

	s.subscribe(c1, "/foo/bar/2")
	// ch3, _ := s.subscribe(c1, "/foo/bar/2")
	// msg = ""
	// go func() { msg = (<-ch3).data }()
	// s.publish(c2, "/foo/bar", "ping")
	// runtime.Gosched()
	// assert(msg == "ping", t, "failed to receive message from new active channel")

	ch4, _ := s.connect(c1)
	// _, ok = <-ch3
	// assert(!ok, t, "new connect overrides other active channel")
	msg = ""
	go func() { msg = (<-ch4).data }()
	s.publish(c2, "/foo/bar/2", "ping")
	runtime.Gosched()
	assert(msg == "ping", t, "failed to receive message from new active connect (got %v)", msg)
}

func TestAvoidReuseClientId(t *testing.T) {
	// log.Println("Testing client ID reuse...")
	s := newServer()
	names := make(map[string]bool)
	var id string
	var err error
	for i := 1; i <= 10000; i++ {
		id, err = s.handshake()
		if _, ok := names[id]; ok || err != nil {
			t.Fatalf("Client IDs should not conflict... for at least 1 million trials.")
		}
		names[id] = true
	}
}
