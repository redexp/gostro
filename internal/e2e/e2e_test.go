// Package e2e contains end-to-end tests: .gostro → generated Go → HTML.
package e2e

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redexp/gostro/internal/gen"
	__gostro "github.com/redexp/gostro/runtime"
)

var update = flag.Bool("update", false, "rewrite generated files and expected HTML")

// Generated files in this package must match what the generator produces.
func TestGeneratedUpToDate(t *testing.T) {
	files, _ := filepath.Glob("*.gostro")
	if len(files) == 0 {
		t.Fatal("no .gostro files")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		want, err := gen.Source(f, src)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out := gen.OutputPath(f)
		if *update {
			os.WriteFile(out, want, 0o644)
			continue
		}
		got, _ := os.ReadFile(out)
		if !bytes.Equal(got, want) {
			t.Errorf("%s is out of date; run go test ./internal/e2e -update and rerun the tests", out)
		}
	}
}

func render(t *testing.T, n __gostro.Node) string {
	t.Helper()
	s, err := __gostro.RenderString(__gostro.NewCtx(context.Background()), n)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(path, []byte(got), 0o644)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update)", err)
	}
	if got != string(want) {
		t.Errorf("%s does not match:\n got: %s\nwant: %s", name, got, want)
	}
}

func TestPage(t *testing.T) {
	html := render(t, Page(PageProps{Posts: []string{"first", "second <script>"}}, nil))
	golden(t, "page.html", html)

	// Styles are registered once despite several cards.
	if n := strings.Count(html, ".card { border"); n != 1 {
		t.Errorf("card CSS emitted %d times", n)
	}
	if !strings.Contains(html, "<head><meta charset=\"utf-8\"><title>Home</title><style>") {
		t.Errorf("styles did not end up in <head>: %s", html)
	}
	if strings.Contains(html, "<script>\"") || strings.Contains(html, "second <script>") {
		t.Errorf("text is not escaped: %s", html)
	}
}

func TestList(t *testing.T) {
	props := ListProps{
		Items: []string{"a", "b<"},
		Mode:  5,
		HTML:  "<i>raw</i>",
		Off:   true,
		Data:  map[string]int{"</script>": 1},
	}
	golden(t, "list.html", render(t, List(props, nil)))

	props.Items = nil
	props.Mode = 1
	props.Off = false
	html := render(t, List(props, nil))
	for _, want := range []string{"<em>empty</em>", "<p>few</p>", "<button type=\"button\">"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %s", want, html)
		}
	}
}

func TestCardSlots(t *testing.T) {
	html := render(t, Card(CardProps{Title: "Café Crème", Href: "javascript:alert(1)"}, nil))
	want := `<article id="café-crème" class="card"><a href="about:invalid#gostro"><h2>Café Crème</h2></a>Empty</article>`
	if html != want {
		t.Errorf("\n got: %s\nwant: %s", html, want)
	}
	tags := []string{"a", "b", "c", "d"}
	html = render(t, Card(CardProps{Title: "x", Tags: tags}, __gostro.Slots{"default": __gostro.RawHTML("<p>body</p>")}))
	if !strings.Contains(html, `class="card card--wide"`) || !strings.Contains(html, "<p>body</p></article>") {
		t.Errorf("slot or class:list: %s", html)
	}
}
