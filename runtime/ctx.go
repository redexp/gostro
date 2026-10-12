package runtime

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"sync"
)

// Ctx is the context of a single render: the registry of component styles and
// scripts, the request's context.Context and arbitrary values.
type Ctx struct {
	context.Context

	mu      sync.Mutex
	styles  registry
	scripts registry
	values  map[any]any
	req     *http.Request
	status  int
}

type registry struct {
	seen  map[string]bool
	items []string
}

func (r *registry) add(key, content string) {
	if r.seen == nil {
		r.seen = map[string]bool{}
	}
	if r.seen[key] {
		return
	}
	r.seen[key] = true
	r.items = append(r.items, content)
}

// NewCtx creates a render context.
func NewCtx(ctx context.Context) *Ctx {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Ctx{Context: ctx}
}

// UseStyle registers component CSS. A block with the same key is added only once.
func (c *Ctx) UseStyle(key, css string) {
	c.mu.Lock()
	c.styles.add(key, css)
	c.mu.Unlock()
}

// UseScript registers component JS. A block with the same key is added only once.
func (c *Ctx) UseScript(key, js string) {
	c.mu.Lock()
	c.scripts.add(key, js)
	c.mu.Unlock()
}

// Styles returns a node that is replaced by all registered styles.
// Since components register styles during rendering (that is, after
// <head>), the node writes a marker that RenderTo/Handler replace at the end.
func (c *Ctx) Styles() Node { return RawHTML(stylesMarker) }

// Scripts returns a node that is replaced by all registered scripts.
func (c *Ctx) Scripts() Node { return RawHTML(scriptsMarker) }

// Set stores an arbitrary value in the render context.
func (c *Ctx) Set(key, value any) {
	c.mu.Lock()
	if c.values == nil {
		c.values = map[any]any{}
	}
	c.values[key] = value
	c.mu.Unlock()
}

// Get returns a value stored with Set.
func (c *Ctx) Get(key any) any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.values[key]
}

// Request returns the HTTP request if rendering was started by Handler.
func (c *Ctx) Request() *http.Request { return c.req }

// SetStatus sets the HTTP response status for Handler (e.g. 404 from a "not found" page).
func (c *Ctx) SetStatus(code int) {
	c.mu.Lock()
	c.status = code
	c.mu.Unlock()
}

// Status returns the status set with SetStatus, or 200.
func (c *Ctx) Status() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status == 0 {
		return http.StatusOK
	}
	return c.status
}

// finalize substitutes the collected styles and scripts for the markers.
func (c *Ctx) finalize(b []byte) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	b = replaceMarker(b, stylesMarker, "style", c.styles.items)
	b = replaceMarker(b, scriptsMarker, "script", c.scripts.items)
	return b
}

func replaceMarker(b []byte, marker, tag string, items []string) []byte {
	if !bytes.Contains(b, []byte(marker)) {
		return b
	}
	var s strings.Builder
	for _, it := range items {
		s.WriteString("<" + tag + ">")
		s.WriteString(it)
		s.WriteString("</" + tag + ">")
	}
	return bytes.ReplaceAll(b, []byte(marker), []byte(s.String()))
}
