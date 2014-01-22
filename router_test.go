package gocomet

import (
	"testing"
)

func TestSimpleRule(t *testing.T) {
	r := newRouter()
	r.add("/meta/handshake", "client1")
	res := r.run("/meta/handshake")
	assert(len(res) == 1 && res[0] == "client1", t, "failed to do simple route")
}

func TestSimpleRules(t *testing.T) {
	r := newRouter()
	id := "client1"
	r.add("/meta/handshake", id)
	r.add("/meta/connect", id)
	res := r.run("/meta/handshake")
	assert(len(res) == 1 && res[0] == id, t, "failed to do simple route")
	res = r.run("/meta/connect")
	assert(len(res) == 1 && res[0] == id, t, "failed to do simple route")
}

func TestMissingMatch(t *testing.T) {
	r := newRouter()
	assert(len(r.run("any")) == 0, t, "nothing matched")
	r.add("/meta/handshake", "client1")
	assert(len(r.run("any")) == 0, t, "nothing matched")
}

func TestWildcardRule(t *testing.T) {
	r := newRouter()
	id := "client1"
	r.add("/foo/*", id)
	assert(len(r.run("any")) == 0, t, "should not match incorrect path")
	res := r.run("/foo/bar")
	assert(len(res) == 1 && res[0] == id, t, "failed to match wildcard path")
	res = r.run("/foo/")
	assert(len(res) == 1 && res[0] == id, t, "failed to match wildcard empty path")
	assert(len(r.run("/foo/bar/")) == 0, t, "should not match more than one path segment with wildcard")
}

func TestWildcardRule2(t *testing.T) {
	r := newRouter()
	id := "client1"
	r.add("/foo/**", id)
	assert(len(r.run("any")) == 0, t, "should not match incorrect path")
	res := r.run("/foo/bar")
	assert(len(res) == 1 && res[0] == id, t, "failed to match wildcard path (got '%v')", res)
	res = r.run("/foo/")
	assert(len(res) == 1 && res[0] == id, t, "failed to match wildcard empty path")
	res = r.run("/foo/bar/")
	assert(len(res) == 1 && res[0] == id, t, "failed to match more than one path segment with wildcard")
}

func TestDuplicateRule(t *testing.T) {
	r := newRouter()
	id := "client1"
	rule1 := r.add("/foo/bar", id)
	rule2 := r.add("/foo/bar", id)
	assert(rule1 == rule2, t, "duplicate rules should be identical")
	res := r.run("/foo/bar")
	assert(len(res) == 1 && res[0] == id, t, "not allow duplicate rules")
	r.add("/foo/*", id)
	res = r.run("/foo/bar")
	assert(len(res) == 1 && res[0] == id, t, "not allow duplicate rules with wildcard")
}

func TestRemoveRule(t *testing.T) {
	r := newRouter()
	assert(len(r.run("/foo")) == 0, t, "no matching rule yet")
	id := "client1"
	rule := r.add("/foo", id)
	if rule == nil {
		t.Fatal("failed to obtain rule reference")
	}
	if rule.router == nil {
		t.Fatal("invalid rule fields")
	}
	res := r.run("/foo")
	assert(len(res) == 1 && res[0] == id, t, "failed to match")
	rule.remove()
	assert(len(r.run("/foo")) == 0, t, "no matching rule now")
}
