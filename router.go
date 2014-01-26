package gocomet

import (
	"bytes"
	"container/list"
	"fmt"
	"strings"
	"sync"
)

/*
A simple router that accepts mutiple matching rules, responds to path
query, and returns the matching client IDs.

Internally, it uses a Trie to do path matching. Adding or removing
rule will change the internal Trie structure. The lookup efficiency
is proportional to the approximte number of path segments.

Note: it's thread-safe and can be shared in different goroutines.
*/
type Router struct {
	*sync.RWMutex
	parent   *Router
	prefix   string
	children map[string]*Router
	rules    map[string]map[string]*Rule
}

func newRouter() *Router {
	return &Router{
		RWMutex:  &sync.RWMutex{},
		children: make(map[string]*Router),
		rules:    make(map[string]map[string]*Rule),
	}
}

/*
Add a new router rule into the table.
*/
func (r *Router) add(path, id string) *Rule {
	if pos := strings.Index(path, "*"); pos > 0 { // wildcard rule
		prefix, part := path[:pos], path[pos:]

		r2, exists := r.obtainSubRouter(prefix)
		if !exists {
			candidates := r.moveSimpleRulesMatching(prefix, r2)
			r2.removeRules(candidates)
		}
		return r2.add(part, id)
	}
	return r.addSimpleRule(path, id)
}

func (r *Router) obtainSubRouter(prefix string) (r2 *Router, ok bool) {
	r.Lock()
	defer r.Unlock()

	r2, ok = r.children[prefix]
	if !ok { // create a new child router
		r2 = newRouter()
		r2.parent = r
		r2.prefix = prefix
		r.children[prefix] = r2
	}
	return
}

func (r *Router) moveSimpleRulesMatching(prefix string, r2 *Router) (candidates []string) {
	r.RLock()
	defer r.RUnlock()

	pos := len(prefix)
	for rp, rules := range r.rules {
		if strings.HasPrefix(rp, prefix) {
			for _, rule := range rules {
				r2.add(rp[pos:], rule.id)
			}
			candidates = append(candidates, rp)
		}
	}
	return
}

func (r *Router) removeRules(rules []string) {
	r.Lock()
	defer r.Unlock()

	for _, rule := range rules {
		delete(r.rules, rule)
	}
}

func (r *Router) addSimpleRule(path, id string) (rule *Rule) {
	r.Lock()
	defer r.Unlock()

	if r.rules[path] == nil {
		r.rules[path] = make(map[string]*Rule)
	}
	if rule, ok := r.rules[path][id]; ok {
		return rule
	}
	rule = &Rule{
		router: r,
		path:   path,
		id:     id,
	}
	r.rules[path][id] = rule
	return
}

/*
Run the router to obtain a list of matched IDs.
*/
func (r *Router) run(path string) (matches []string) {
	matches = r.collectRules(matches, path)
	if !strings.Contains(path, "/") { // try wildcard match
		matches = r.collectRules(matches, "*")
	}
	matches = r.collectRules(matches, "**")

	if len(matches) == 0 { // try sub routers
		for prefix, r2 := range r.children {
			if strings.HasPrefix(path, prefix) {
				matches = r2.run(path[len(prefix):])
				if len(matches) > 0 {
					break
				}
			}
		}
	}
	return
}

func (r *Router) collectRules(matches []string, patt string) []string {
	r.RLock()
	defer r.RUnlock()

	if rules, ok := r.rules[patt]; ok {
		for _, rule := range rules {
			matches = append(matches, rule.id)
		}
	}
	return matches
}

func (r *Router) removeRule(rule *Rule) {
	r.Lock()
	defer r.Unlock()

	if rules, ok := r.rules[rule.path]; ok {
		// remove rule from its router
		delete(rules, rule.id)
	}
}

func (r *Router) minify() {
	parent := r.parent
	if r.hasSubRouters() || r.hasWildcardRules() || parent == nil {
		return
	}

	r.Lock()
	defer r.Unlock()

	// if no child router and no other wildcard rule,
	// merge current router to its parent
	if parent != nil {
		for _, rules := range r.rules {
			for path, rule := range rules {
				parent.add(r.prefix+path, rule.id)
				rule.router = nil // ease GC work
			}
		}
		parent.removeSubRouter(r.prefix)
	}
}

func (r *Router) hasSubRouters() bool {
	r.RLock()
	defer r.RUnlock()
	return len(r.children) > 0
}

func (r *Router) hasWildcardRules() bool {
	r.RLock()
	defer r.RUnlock()
	_, ok := r.rules["*"]
	if !ok {
		_, ok = r.rules["**"]
	}
	return ok
}

func (r *Router) removeSubRouter(prefix string) {
	r.Lock()
	defer r.Unlock()
	delete(r.children, prefix)
}

func (r *Router) String() string {
	var buf bytes.Buffer
	r.toString(&buf, 0)
	return buf.String()
}

func (r *Router) toString(buf *bytes.Buffer, tabSize int) {
	r.RLock()
	defer r.RUnlock()

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
	router *Router
	path   string
	id     string
}

func (rule *Rule) remove() {
	rule.router.removeRule(rule)
	if strings.HasPrefix(rule.path, "*") {
		// minify the router table if wildcard rule is removed
		rule.router.minify()
	}
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
