package gocomet

import (
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"strings"
	"sync"
)

/*
A simple router that accepts mutiple matching rules, responds to path
query, and execute registered callback.

Internally, it uses a Trie to do path matching. Adding or removing
rule will change the internal Trie structure. The lookup efficiency
is proportional to the approximte number of path segments.

Note: it's thread-safe and can be shared in different goroutines. But
the callback should not hold CPU for too long.
*/
type Router struct {
	*sync.RWMutex
	parent   *Router
	prefix   string
	children map[string]*Router
	rules    map[string]map[*Rule]bool
}

func newRouter() *Router {
	return &Router{
		RWMutex:  &sync.RWMutex{},
		children: make(map[string]*Router),
		rules:    make(map[string]map[*Rule]bool),
	}
}

func (r *Router) add(path string, callback func(string)) *Rule {
	if pos := strings.Index(path, "*"); pos > 0 { // wildcard rule
		prefix, part := path[:pos], path[pos:]
		r2, ok := r.children[prefix]
		if !ok { // create a new child router
			r2 = newRouter()
			r2.parent = r
			r2.prefix = prefix
			r.children[prefix] = r2

			// move simple rules that match the prefix to child router
			var candidates []string
			for rp, rules := range r.rules {
				if strings.HasPrefix(rp, prefix) {
					for rule, _ := range rules {
						r2.add(rp[pos:], rule.callback)
					}
					candidates = append(candidates, rp)
				}
			}

			// remove them from the current router
			for _, candidate := range candidates {
				delete(r.rules, candidate)
			}
		}
		return r2.add(part, callback)
	}
	// simple rule
	rule := &Rule{
		router:   r,
		path:     path,
		callback: callback,
	}
	if r.rules[path] == nil {
		r.rules[path] = make(map[*Rule]bool)
	}
	r.rules[path][rule] = true
	return rule
}

func (r *Router) run(path string) bool {
	return r.runStep(path, path)
}

func (r *Router) runStep(path, originalPath string) (matched bool) {
	if rules, ok := r.rules[path]; ok { // match simple rules
		matched = true
		invokeAll(rules, originalPath)
	}
	if !strings.Contains(path, "/") { // try wildcard match
		if rules, ok := r.rules["*"]; ok {
			matched = true
			invokeAll(rules, originalPath)
		}
	}
	if rules, ok := r.rules["**"]; ok {
		matched = true
		invokeAll(rules, originalPath)
	}
	if !matched { // try sub routers
		for prefix, r2 := range r.children {
			if strings.HasPrefix(path, prefix) && r2.runStep(path[len(prefix):], originalPath) {
				matched = true
				break
			}
		}
	}
	return
}

func invokeAll(rules map[*Rule]bool, path string) {
	for rule, _ := range rules {
		rule.callback(path)
	}
}

func (r *Router) String() string {
	var buf bytes.Buffer
	r.toString(&buf, 0)
	return buf.String()
}

func (r *Router) toString(buf *bytes.Buffer, tabSize int) {
	tab := makeTab(tabSize)
	buf.WriteString("Router(\n")
	for part, _ := range r.rules {
		fmt.Fprintf(buf, "%v  %v\n", tab, part)
	}
	for part, child := range r.children {
		fmt.Fprintf(buf, "%v  %v -> ", tab, part)
		child.toString(buf, tabSize+2)
		buf.WriteRune('\n')
	}
	fmt.Fprintf(buf, "%v)", tab)
}

func makeTab(tabSize int) string {
	tabSlice := make([]rune, tabSize)
	for i, _ := range tabSlice {
		tabSlice[i] = ' '
	}
	return string(tabSlice)
}

type Rule struct {
	router   *Router
	path     string
	callback func(string)
}

func (rule *Rule) remove() error {
	rules, ok := rule.router.rules[rule.path]
	if ok {
		_, ok = rules[rule]
	}
	if !ok {
		return errors.New(fmt.Sprintf("Rule '%v' is not found.", rule))
	}
	// remove rule from its router
	delete(rules, rule)

	if len(rule.router.children) > 0 {
		return nil // skip if has child router
	}
	for r, _ := range rules {
		if strings.HasPrefix(r.path, "*") {
			return nil // skip if has other wildcard rule
		}
	}

	// if no child router and no other wildcard rule,
	// merge current router to its parent
	parent := rule.router.parent
	if parent != nil {
		for r, _ := range rules {
			parent.add(rule.router.prefix+r.path, rule.callback)
			r.router = nil // ease GC work
		}
		delete(parent.children, rule.router.prefix)
	}
	return nil
}

func (rule *Rule) String() string {
	stack := list.New()
	stack.PushFront(rule.path)
	for r := rule.router; r != nil; r = r.parent {
		stack.PushFront(r.prefix)
	}
	var b bytes.Buffer
	for e := stack.Front(); e != nil; e = e.Next() {
		b.WriteString(e.Value.(string))
	}
	return b.String()
}
