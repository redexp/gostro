package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/redexp/gostro/internal/gen"
	gostro "github.com/redexp/gostro/runtime"
)

type watchOptions struct {
	patterns []string
	verbose  bool
	cmd      string // app command restarted after changes
	sse      string // address of the reload events SSE server ("" disables it)
}

func runWatch(args []string) error {
	fl := flag.NewFlagSet("watch", flag.ExitOnError)
	o := watchOptions{}
	fl.BoolVar(&o.verbose, "v", false, "verbose output")
	fl.StringVar(&o.cmd, "cmd", "", "app command to restart after changes, e.g. \"go run ./examples/blog\"")
	fl.StringVar(&o.sse, "sse", ":7331", "SSE server address for page live reload (empty disables it)")
	fl.Parse(args)
	o.patterns = fl.Args()
	return watchLoop(o)
}

func watchLoop(o watchOptions) error {
	files, err := findFiles(o.patterns)
	if err != nil {
		return err
	}
	generateAll(files, o.verbose, true)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	for _, root := range watchRoots(o.patterns) {
		if err := addRecursive(w, root.dir, root.recursive); err != nil {
			return err
		}
	}

	hub := newHub()
	env := os.Environ()
	if o.sse != "" {
		ln, err := net.Listen("tcp", o.sse)
		if err != nil {
			return fmt.Errorf("SSE server: %w", err)
		}
		go http.Serve(ln, hub)
		url := "http://" + sseHost(ln.Addr().String()) + "/events"
		env = append(env, gostro.LiveReloadEnv+"="+url)
		logf("live reload: %s (add {__gostro.LiveReload()} to your layout)", url)
	}

	var app *process
	startApp := func() {
		if o.cmd == "" {
			return
		}
		app, err = startProcess(o.cmd, env)
		if err != nil {
			logf("start %q: %v", o.cmd, err)
		}
	}
	startApp()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	pendingGostro := map[string]bool{}
	pendingRestart := false
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	logf("watching for changes… (Ctrl+C to exit)")

	for {
		select {
		case <-sig:
			if app != nil {
				app.stop()
			}
			return nil

		case err := <-w.Errors:
			logf("fsnotify: %v", err)

		case ev := <-w.Events:
			path := ev.Name
			if ev.Has(fsnotify.Create) {
				if info, err := os.Stat(path); err == nil && info.IsDir() && !skipDir(info.Name()) {
					addRecursive(w, path, true)
					continue
				}
			}
			switch {
			case strings.HasSuffix(path, ".gostro"):
				pendingGostro[path] = true
			case strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_gostro.go") && !strings.HasSuffix(path, "_test.go"):
				pendingRestart = true
			default:
				continue
			}
			timer.Reset(100 * time.Millisecond)

		case <-timer.C:
			ok := true
			for path := range pendingGostro {
				delete(pendingGostro, path)
				if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
					// .gostro was removed: remove the generated file.
					if os.Remove(gen.OutputPath(path)) == nil {
						logf("removed %s", gen.OutputPath(path))
						pendingRestart = true
					}
					continue
				}
				res, err := gen.GenerateFile(path)
				if err != nil {
					printError(err)
					ok = false
					continue
				}
				if res.Changed {
					logf("generated %s", res.Out)
					pendingRestart = true
				}
			}
			if !ok || !pendingRestart {
				continue
			}
			pendingRestart = false
			if app != nil {
				app.stop()
			}
			startApp()
			hub.broadcast("reload")
		}
	}
}

type watchRoot struct {
	dir       string
	recursive bool
}

func watchRoots(patterns []string) []watchRoot {
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	var roots []watchRoot
	for _, pat := range patterns {
		if dir, ok := strings.CutSuffix(pat, "..."); ok {
			dir = strings.TrimSuffix(dir, "/")
			if dir == "" {
				dir = "."
			}
			roots = append(roots, watchRoot{dir, true})
			continue
		}
		if info, err := os.Stat(pat); err == nil && !info.IsDir() {
			roots = append(roots, watchRoot{filepath.Dir(pat), false})
			continue
		}
		roots = append(roots, watchRoot{pat, false})
	}
	return roots
}

func addRecursive(w *fsnotify.Watcher, root string, recursive bool) error {
	if !recursive {
		return w.Add(root)
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path != root && skipDir(d.Name()) {
			return filepath.SkipDir
		}
		return w.Add(path)
	})
}

func sseHost(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "::" || host == "0.0.0.0" {
		host = "localhost"
	}
	return net.JoinHostPort(host, port)
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gostro: "+format+"\n", args...)
}

// --- SSE --------------------------------------------------------------------

// hub broadcasts reload events to connected pages.
type hub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

func newHub() *hub { return &hub{clients: map[chan string]struct{}{}} }

func (h *hub) broadcast(event string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- event:
		default:
		}
	}
}

func (h *hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/events" {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	hdr := w.Header()
	hdr.Set("Content-Type", "text/event-stream")
	hdr.Set("Cache-Control", "no-cache")
	hdr.Set("Access-Control-Allow-Origin", "*")
	c := make(chan string, 1)
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
	}()
	fmt.Fprint(w, ": gostro\n\n")
	flusher.Flush()
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-c:
			fmt.Fprintf(w, "event: %s\ndata: 1\n\n", ev)
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// --- app process ------------------------------------------------------------

type process struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func startProcess(command string, env []string) (*process, error) {
	c := shellCommand(command)
	c.Stdout, c.Stderr, c.Stdin = os.Stdout, os.Stderr, nil
	c.Env = env
	if err := c.Start(); err != nil {
		return nil, err
	}
	p := &process{cmd: c, done: make(chan struct{})}
	go func() {
		c.Wait()
		close(p.done)
	}()
	return p, nil
}

// stop terminates the process along with its children (go run starts a separate binary).
func (p *process) stop() {
	select {
	case <-p.done:
		return
	default:
	}
	killTree(p.cmd)
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		p.cmd.Process.Kill()
	}
}
