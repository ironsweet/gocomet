package gocomet

import (
	"testing"
)

func TestSimpleRule(t *testing.T) {
	r := newRouter()
	var matched string
	r.add("/meta/handshake", func(path string) {
		matched = path
	})
	r.run("/meta/handshake")
	assert(matched == "/meta/handshake", t, "failed to do simple route")
}

func TestSimpleRules(t *testing.T) {
	r := newRouter()
	var matched string
	f := func(path string) { matched = path }
	r.add("/meta/handshake", f)
	r.add("/meta/connect", f)
	r.run("/meta/handshake")
	assert(matched == "/meta/handshake", t, "failed to do simple route")
	r.run("/meta/connect")
	assert(matched == "/meta/connect", t, "failed to do simple route")
}

func TestMissingMatch(t *testing.T) {
	r := newRouter()
	r.run("any") // nothing happens
	r.add("/meta/handshake", func(string) {})
	r.run("any") // still nothing happens
}

func TestWildcardRule(t *testing.T) {
	r := newRouter()
	var matched string
	f := func(path string) { matched = path }
	r.add("/foo/*", f)
	r.run("any")
	assert(matched == "", t, "should not match incorrect path")
	r.run("/foo/bar")
	assert(matched == "/foo/bar", t, "failed to match wildcard path")
	r.run("/foo/")
	assert(matched == "/foo/", t, "failed to match wildcard empty path")
	matched = ""
	r.run("/foo/bar/")
	assert(matched == "", t, "should not match more than one path segment with wildcard")
}

func TestWildcardRule2(t *testing.T) {
	r := newRouter()
	var matched string
	f := func(path string) { matched = path }
	r.add("/foo/**", f)
	// r.run("any")
	// assert(matched == "", t, "should not match incorrect path")
	r.run("/foo/bar")
	assert(matched == "/foo/bar", t, "failed to match wildcard path (got '%v')", matched)
	r.run("/foo/")
	assert(matched == "/foo/", t, "failed to match wildcard empty path")
	matched = ""
	r.run("/foo/bar/")
	assert(matched == "/foo/bar/", t, "failed to match more than one path segment with wildcard")
}

func TestMultipleMatch(t *testing.T) {
	r := newRouter()
	var nHit int = 0
	f := func(path string) { nHit++ }
	r.add("/foo/bar", f)
	r.add("/foo/bar", f)
	r.run("/foo/bar")
	assert(nHit == 2, t, "should allow duplicate rules")
	nHit = 0
	r.add("/foo/*", f)
	r.run("/foo/bar")
	assert(nHit == 3, t, "should allow mutiple match")
}

func TestRemoveRule(t *testing.T) {
	r := newRouter()
	var matched bool
	f := func(string) { matched = true }
	r.run("/foo")
	assert(!matched, t, "no matching rule yet")
	rule := r.add("/foo", f)
	if rule == nil {
		t.Fatal("failed to obtain rule reference")
	}
	if rule.router == nil {
		t.Fatal("invalid rule fields")
	}
	r.run("/foo")
	assert(matched, t, "failed to match")
	assert(rule.remove() == nil, t, "failed to remove rule")
	matched = false
	r.run("/foo")
	assert(!matched, t, "no matching rule now")
}
