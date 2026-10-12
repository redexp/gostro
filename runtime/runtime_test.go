package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func capture(f func(w Writer)) (string, error) {
	var b strings.Builder
	w := NewWriter(&b)
	f(w)
	return b.String(), w.Err()
}

type stringer struct{}

func (stringer) String() string { return "<s>" }

type custom struct{}

func TestText(t *testing.T) {
	tests := []struct {
		v    any
		want string
	}{
		{"<script>alert('x')</script>", "&lt;script&gt;alert(&#39;x&#39;)&lt;/script&gt;"},
		{`"&"`, "&#34;&amp;&#34;"},
		{42, "42"},
		{int64(-7), "-7"},
		{uint8(255), "255"},
		{3.25, "3.25"},
		{float32(0.5), "0.5"},
		{true, "true"},
		{nil, ""},
		{stringer{}, "&lt;s&gt;"},
		{errors.New("a<b"), "a&lt;b"},
		{RawHTML("<b>café</b>"), "<b>café</b>"},
		{SafeURL("<x>"), "&lt;x&gt;"},
	}
	for _, tt := range tests {
		got, err := capture(func(w Writer) { Text(w, tt.v) })
		if err != nil || got != tt.want {
			t.Errorf("Text(%#v) = %q, %v; want %q", tt.v, got, err, tt.want)
		}
	}
	_, err := capture(func(w Writer) { Text(w, custom{}) })
	if err == nil || !strings.Contains(err.Error(), "runtime.custom") {
		t.Errorf("unsupported type: %v", err)
	}
}

func TestAttrEscaping(t *testing.T) {
	got, _ := capture(func(w Writer) { Attr(w, `" onmouseover="alert(1)`) })
	if got != "&#34; onmouseover=&#34;alert(1)" {
		t.Errorf("Attr: %q", got)
	}
	got, _ = capture(func(w Writer) { Attr(w, `' onfocus='x`) })
	if strings.ContainsAny(got, `'"`) {
		t.Errorf("Attr left quotes unescaped: %q", got)
	}
}

// XSS vectors for URL attributes.
func TestURLSecurity(t *testing.T) {
	unsafe := []string{
		"javascript:alert(1)",
		"JaVaScRiPt:alert(1)",
		"  javascript:alert(1)",
		"\tjavascript:alert(1)",
		"java\tscript:alert(1)",
		"java\nscript:alert(1)",
		"java\rscript:alert(1)",
		"\x00javascript:alert(1)",
		"\x01\x02javascript:alert(1)",
		"vbscript:msgbox(1)",
		"VBSCRIPT:msgbox(1)",
		"data:text/html,<script>alert(1)</script>",
		"DATA:text/html;base64,PHNjcmlwdD4=",
		"da\tta:text/html,x",
		"data:application/javascript,alert(1)",
	}
	for _, u := range unsafe {
		got, _ := capture(func(w Writer) { URL(w, u) })
		if got != UnsafeURLReplacement {
			t.Errorf("URL(%q) = %q: unsafe URL let through", u, got)
		}
	}
	safe := map[string]string{
		"https://example.com/?a=1&b=2": "https://example.com/?a=1&amp;b=2",
		"/path/to":                     "/path/to",
		"relative/javascript:x":        "relative/javascript:x",
		"#anchor":                      "#anchor",
		"?q=javascript:x":              "?q=javascript:x",
		"mailto:a@b.c":                 "mailto:a@b.c",
		"data:image/png;base64,iVBOR":  "data:image/png;base64,iVBOR",
		"DATA:IMAGE/GIF;base64,R0lG":   "DATA:IMAGE/GIF;base64,R0lG",
		`https://x/"><script>`:         "https://x/&#34;&gt;&lt;script&gt;",
	}
	for u, want := range safe {
		got, _ := capture(func(w Writer) { URL(w, u) })
		if got != want {
			t.Errorf("URL(%q) = %q, want %q", u, got, want)
		}
	}
	got, _ := capture(func(w Writer) { URL(w, SafeURL("javascript:void(0)")) })
	if got != "javascript:void(0)" {
		t.Errorf("SafeURL: %q", got)
	}
}

func TestJSON(t *testing.T) {
	got := JSON(map[string]string{"x": "</script><script>alert(1)</script>", "y": " &'"})
	s := string(got)
	if strings.Contains(s, "</script>") || strings.Contains(s, "<") || strings.Contains(s, "&") || strings.Contains(s, " ") {
		t.Errorf("JSON is not safe for <script>: %s", s)
	}
}

func TestClassList(t *testing.T) {
	got, _ := capture(func(w Writer) {
		ClassList(w, []any{"a", "", nil, []string{"b", "c"}, map[string]bool{"z": true, "y": true, "off": false}, `"x"`})
	})
	if got != "a b c y z &#34;x&#34;" {
		t.Errorf("ClassList: %q", got)
	}
}

func TestBoolAttr(t *testing.T) {
	for v, want := range map[any]string{true: " disabled", false: "", "x": ` disabled="x"`} {
		got, _ := capture(func(w Writer) { BoolAttr(w, " disabled", v) })
		if got != want {
			t.Errorf("BoolAttr(%v) = %q", v, got)
		}
	}
	got, _ := capture(func(w Writer) { BoolAttr(w, " disabled", nil) })
	if got != "" {
		t.Errorf("BoolAttr(nil) = %q", got)
	}
}

func TestChild(t *testing.T) {
	ctx := NewCtx(context.Background())
	n := NodeFunc(func(ctx *Ctx, w Writer) error { w.WriteString("<i>"); return nil })
	got, err := capture(func(w Writer) {
		Child(ctx, w, n)
		Child(ctx, w, []Node{n, RawHTML("<b>")})
		Child(ctx, w, "<")
		Child(ctx, w, nil)
		Child(ctx, w, NodeFunc(nil))
	})
	if err != nil || got != "<i><i><b>&lt;" {
		t.Errorf("Child: %q, %v", got, err)
	}
}

func TestWriterStopsOnError(t *testing.T) {
	ctx := NewCtx(context.Background())
	boom := errors.New("boom")
	got, err := capture(func(w Writer) {
		Child(ctx, w, "a")
		Render(ctx, w, NodeFunc(func(*Ctx, Writer) error { return boom }))
		Child(ctx, w, "b")
	})
	if !errors.Is(err, boom) || got != "a" {
		t.Errorf("%q, %v", got, err)
	}
}

func TestSlots(t *testing.T) {
	ctx := NewCtx(context.Background())
	fallback := RawHTML("fallback")
	s := Slots{"default": RawHTML("body")}
	got, _ := capture(func(w Writer) {
		s.RenderOr(ctx, w, "default", fallback)
		s.RenderOr(ctx, w, "header", fallback)
		s.RenderOr(ctx, w, "footer", nil)
		Slots(nil).RenderOr(ctx, w, "default", fallback)
	})
	if got != "bodyfallbackfallback" {
		t.Errorf("Slots: %q", got)
	}
}

func TestStylesDedup(t *testing.T) {
	page := NodeFunc(func(ctx *Ctx, w Writer) error {
		w.WriteString("<head>")
		Render(ctx, w, ctx.Styles())
		w.WriteString("</head><body>")
		for i := 0; i < 100; i++ {
			ctx.UseStyle("card#0", ".card{}")
			ctx.UseScript("card#0", "init()")
		}
		ctx.UseStyle("btn#0", ".btn{}")
		Render(ctx, w, ctx.Scripts())
		w.WriteString("</body>")
		return w.Err()
	})
	got, err := RenderString(nil, page)
	want := "<head><style>.card{}</style><style>.btn{}</style></head><body><script>init()</script></body>"
	if err != nil || got != want {
		t.Errorf("\n got: %s\nwant: %s", got, want)
	}
}

func TestHandler(t *testing.T) {
	h := Handler(func(r *http.Request) Node {
		return NodeFunc(func(ctx *Ctx, w Writer) error {
			Text(w, ctx.Request().URL.Query().Get("q"))
			return nil
		})
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/?q=<x>", nil))
	if rec.Code != 200 || rec.Body.String() != "&lt;x&gt;" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Errorf("%d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}

	fail := Handler(func(r *http.Request) Node {
		return NodeFunc(func(ctx *Ctx, w Writer) error {
			w.WriteString("partial")
			return fmt.Errorf("failure")
		})
	})
	rec = httptest.NewRecorder()
	fail.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "partial") {
		t.Errorf("render error: %d %q", rec.Code, rec.Body.String())
	}
}

func TestLiveReload(t *testing.T) {
	t.Setenv(LiveReloadEnv, "")
	if s, _ := RenderString(nil, LiveReload()); s != "" {
		t.Errorf("outside watch: %q", s)
	}
	t.Setenv(LiveReloadEnv, "http://localhost:7331/events")
	if s, _ := RenderString(nil, LiveReload()); !strings.Contains(s, `EventSource("http://localhost:7331/events")`) {
		t.Errorf("watch: %q", s)
	}
}
