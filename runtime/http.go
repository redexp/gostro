package runtime

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"strconv"
)

// Handler returns an http.Handler that renders the page returned by build.
// Output is buffered: on a render error the client gets a 500 instead of
// truncated HTML. Templates can access the request via ctx.Request() and set
// the response status via ctx.SetStatus(code).
func Handler(build func(*http.Request) Node) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := NewCtx(r.Context())
		ctx.req = r
		var buf bytes.Buffer
		if err := RenderTo(ctx, &buf, build(r)); err != nil {
			log.Printf("gostro: render %s: %v", r.URL.Path, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		h := w.Header()
		if h.Get("Content-Type") == "" {
			h.Set("Content-Type", "text/html; charset=utf-8")
		}
		h.Set("Content-Length", strconv.Itoa(buf.Len()))
		w.WriteHeader(ctx.Status())
		w.Write(buf.Bytes())
	})
}

// LiveReloadEnv is the environment variable `gostro watch` sets for the app it
// runs: the address of the reload events SSE stream.
const LiveReloadEnv = "GOSTRO_LIVERELOAD"

// LiveReload returns the page live-reload script for `gostro watch` mode.
// Outside of it the node renders nothing, so it is safe to keep in the layout.
func LiveReload() Node {
	return NodeFunc(func(ctx *Ctx, w Writer) error {
		url := os.Getenv(LiveReloadEnv)
		if url == "" {
			return nil
		}
		w.WriteString(`<script>(function(){var es=new EventSource(`)
		w.WriteString(string(JSON(url)))
		w.WriteString(`);es.addEventListener("reload",function(){es.close();(function wait(){fetch(location.href,{method:"HEAD",cache:"no-store"}).then(function(r){if(r.ok)location.reload();else setTimeout(wait,200)},function(){setTimeout(wait,200)})})()})})();</script>`)
		return nil
	})
}
