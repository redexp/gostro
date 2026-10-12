package parse

import (
	"errors"
	"strings"
	"testing"

	"github.com/redexp/gostro/internal/ast"
)

func mustParse(t *testing.T, src string) *ast.File {
	t.Helper()
	f, err := ParseFile("t.gostro", "t.gostro", []byte(src))
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	return f
}

func parseErr(t *testing.T, src string) *Error {
	t.Helper()
	_, err := ParseFile("t.gostro", "t.gostro", []byte(src))
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("%q: expected an error, got %v", src, err)
	}
	return e
}

func TestClauseCompositeLiterals(t *testing.T) {
	tests := []struct {
		src    string
		clause string
	}{
		{"for _, x := range []int{1, 2} {\n}", "for _, x := range []int{1, 2}"},
		{"for k := range map[string]int{\"a\": 1} {\n}", "for k := range map[string]int{\"a\": 1}"},
		{"for _, x := range map[string][]int{} {\n}", "for _, x := range map[string][]int{}"},
		{"for _, x := range []struct{ A int }{{1}} {\n}", "for _, x := range []struct{ A int }{{1}}"},
		{"for _, x := range [2]pkg.T{} {\n}", "for _, x := range [2]pkg.T{}"},
		{"if m[k] {\n}", "if m[k]"},
		{"if xs[0] > 1 {\n}", "if xs[0] > 1"},
		{"if ok := f(T{1}); ok {\n}", "if ok := f(T{1}); ok"},
		{"for i := 0; i < n; i++ {\n}", "for i := 0; i < n; i++"},
		{"for {\n}", "for"},
	}
	for _, tt := range tests {
		f := mustParse(t, tt.src)
		ctl, ok := f.Body[0].(*ast.Control)
		if !ok {
			t.Fatalf("%q: %T", tt.src, f.Body[0])
		}
		if got := ctl.Clause.String(); got != tt.clause {
			t.Errorf("%q: clause %q, want %q", tt.src, got, tt.clause)
		}
	}
}

func TestElseChain(t *testing.T) {
	f := mustParse(t, "if a {\n<b/>\n} else if c {\n<i/>\n} else {\n<u/>\n}")
	ctl := f.Body[0].(*ast.Control)
	if !ctl.HasElse || !ctl.ElseIf {
		t.Fatal("no else if")
	}
	inner := ctl.Else[0].(*ast.Control)
	if !inner.HasElse || inner.ElseIf || len(inner.Else) != 1 {
		t.Fatalf("invalid else: %+v", inner)
	}
	// else on a new line is no longer an else (as in Go): it is an error or a separate statement.
	f = mustParse(t, "if a {\n<b/>\n}\nx := 1\n_ = x")
	if f.Body[0].(*ast.Control).HasElse || len(f.Body) != 3 {
		t.Fatal("unexpected else")
	}
}

func TestStatementsInMarkup(t *testing.T) {
	f := mustParse(t, "<div>\n  n := len(xs)\n  x++\n  <span>{n}</span>\n  fmt.Println(\"a\")\n</div>")
	el := f.Body[0].(*ast.Element)
	var kinds []string
	for _, c := range el.Children {
		switch c := c.(type) {
		case *ast.GoChunk:
			kinds = append(kinds, "go:"+c.Code.String())
		case *ast.Element:
			kinds = append(kinds, "<"+c.Name+">")
		}
	}
	want := "go:n := len(xs)|go:x++|<span>|go:fmt.Println(\"a\")"
	if got := strings.Join(kinds, "|"); got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
}

func TestTagsInExpressions(t *testing.T) {
	f := mustParse(t, "xs := []Node{<li/>, <li>{\"a\"}</li>}\ny := f(<b/>, <Card title=\"x\"/>)")
	if n := countTags(f.Body[0].(*ast.GoChunk).Code); n != 2 {
		t.Errorf("tags in slice: %d", n)
	}
	code := f.Body[1].(*ast.GoChunk).Code
	if n := countTags(code); n != 2 {
		t.Errorf("tags in arguments: %d", n)
	}
	if _, ok := code.Parts[3].Tag.(*ast.Component); !ok {
		t.Errorf("expected a component, got %T", code.Parts[3].Tag)
	}
}

func countTags(c ast.Code) int {
	n := 0
	for _, p := range c.Parts {
		if p.Tag != nil {
			n++
		}
	}
	return n
}

func TestNoTagsInStringsAndComments(t *testing.T) {
	f := mustParse(t, "s := \"<div>\"\nr := `<p>`\n// <b>\n/* <i> */\nm := map[string]int{\"a<b\": 1}\nok := a < b")
	for _, n := range f.Body {
		if countTags(n.(*ast.GoChunk).Code) != 0 {
			t.Errorf("spurious tag in %q", n.(*ast.GoChunk).Code.String())
		}
	}
}

func TestComponentsAndSlots(t *testing.T) {
	f := mustParse(t, "<ui.Card title=\"x\" max-width={10} wide>\n<h2 slot=\"header\">{\"a\"}</h2>\n<p/>\n<Badge slot=\"footer\"/>\n</ui.Card>")
	c := f.Body[0].(*ast.Component)
	if c.Pkg != "ui" || c.Name != "Card" {
		t.Fatalf("%s.%s", c.Pkg, c.Name)
	}
	if got := strings.Join(c.SlotOrder, ","); got != "header,default,footer" {
		t.Errorf("slots: %s", got)
	}
	h2 := c.Slots["header"][0].(*ast.Element)
	if len(h2.Attrs) != 0 {
		t.Errorf("slot attribute not removed: %+v", h2.Attrs)
	}
}

func TestFuncKinds(t *testing.T) {
	f := mustParse(t, `
type BadgeProps struct{ Text string }

func Badge(props BadgeProps) {
	<span>{props.Text}</span>
}

func helper(x int) int {
	{
		x++
	}
	return x
}

func item(s string) Node {
	return <li>{s}</li>
}

func (r *T) Method() {}
`)
	if len(f.Funcs) != 4 {
		t.Fatalf("funcs: %d", len(f.Funcs))
	}
	if !f.Funcs[0].Component || !f.Funcs[0].AddSlots {
		t.Error("Badge must be a component with slots")
	}
	for _, fn := range f.Funcs[1:] {
		if fn.Component {
			t.Errorf("%s is not a component", fn.Name)
		}
	}
	if len(f.PropsTypes) != 1 || f.PropsTypes[0].Name != "BadgeProps" {
		t.Errorf("BadgeProps not hoisted: %+v", f.PropsTypes)
	}
	if f.HasBody {
		t.Error("no implicit body")
	}
}

func TestPropsHoisting(t *testing.T) {
	f := mustParse(t, "type CardProps struct{ A int }\ntype Props struct{ B int }\n<p/>")
	if len(f.PropsTypes) != 1 || f.PropsTypes[0].Name != "CardProps" {
		t.Fatalf("hoisted: %+v", f.PropsTypes)
	}
	// type Props is not special: it stays in the body as a regular statement.
	if c, ok := f.Body[0].(*ast.GoChunk); !ok || c.Code.String() != "type Props struct{ B int }" {
		t.Fatalf("type Props: %#v", f.Body[0])
	}
}

func TestLabels(t *testing.T) {
	f := mustParse(t, "outer:\nfor _, a := range xs {\n  for _, b := range a {\n    if b {\n      continue outer\n    }\n    <i/>\n  }\n}")
	if c, ok := f.Body[0].(*ast.GoChunk); !ok || c.Code.String() != "outer:" {
		t.Fatalf("label: %#v", f.Body[0])
	}
}

func TestBareTextDetection(t *testing.T) {
	bad := []string{
		"<p>Hello</p>",
		"<p>Hello world</p>",
		"<p>\n  Hello\n</p>",
		"<p>Price: 5</p>",
		"<p>5 items</p>",
		"<p>(beta)</p>",
		"<p>-</p>",
		"<p>Don't</p>",
		"<p>\"text\"</p>",
		"<p>Ok!</p>",
	}
	for _, src := range bad {
		e := parseErr(t, src)
		if !strings.Contains(e.Msg, "bare text") {
			t.Errorf("%q: %v", src, e)
		}
	}
	good := []string{
		"<p>{\"Hello\"}</p>",
		"<p>\n  x := 1\n  _ = x\n</p>",
		"<p>\n  var x int\n  _ = x\n</p>",
		"<p>\n  f := func() int { return 1 }\n  _ = f\n</p>",
		"<p>\n  ch <- 1\n</p>",
		"<p>\n  xs := []Node{}\n  _ = xs\n</p>",
	}
	for _, src := range good {
		mustParse(t, src)
	}
}

func TestRawText(t *testing.T) {
	f := mustParse(t, "<style>a > b { c: d }</style>\n<style is:inline>x{}</style>\n<script>if (a < b) { f() }</script>\n<script src=\"x.js\"></script>\n<script>let d = {__gostro.JSON(v)}</script>")
	s0 := f.Body[0].(*ast.StyleBlock)
	if s0.Inline || s0.CSS != "a > b { c: d }" || s0.Key != "t.gostro#0" {
		t.Errorf("style: %+v", s0)
	}
	if !f.Body[1].(*ast.StyleBlock).Inline {
		t.Error("is:inline")
	}
	sc := f.Body[2].(*ast.ScriptBlock)
	if sc.Inline || sc.JS() != "if (a < b) { f() }" || sc.Key != "t.gostro#0" {
		t.Errorf("script: %+v", sc)
	}
	if !f.Body[3].(*ast.ScriptBlock).Inline {
		t.Error("script with attributes must be inline")
	}
	dyn := f.Body[4].(*ast.ScriptBlock)
	if !dyn.Inline || len(dyn.Parts) != 2 || dyn.Parts[1].Expr == nil {
		t.Errorf("JSON insertion: %+v", dyn.Parts)
	}
}

func TestPrettyError(t *testing.T) {
	e := parseErr(t, "<div>\n  <p>Héllo</p>\n</div>")
	want := "t.gostro:2:6: bare text is not allowed; wrap it in {\"…\"}\n 2 |   <p>Héllo</p>\n   |      ^^^^^\n hint: {\"Héllo\"}"
	if got := e.Pretty(); got != want {
		t.Errorf("\n got: %s\nwant: %s", got, want)
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"<div>{x}</div>", "if a {\n<b/>\n} else {\n<i/>\n}", "x := <p>{\"a\"}</p>",
		"<Card title=\"x\"><h2 slot=\"h\"/></Card>", "<script>{__gostro.JSON(x)}</script>",
		"switch x {\ncase 1:\n<a/>\n}", "func F(p FProps) {\n<b/>\n}", "<!DOCTYPE html>",
		"<style>a{}</style>", "<p>Hello</p>", "{", "<", "</", "<a b={", "<a b=\"",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		// Must not panic; errors must be *Error only.
		_, err := ParseFile("f.gostro", "f.gostro", []byte(src))
		if err != nil {
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("error is not *Error: %v", err)
			}
		}
	})
}
