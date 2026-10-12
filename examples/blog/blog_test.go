package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, path string) (int, string) {
	t.Helper()
	srv := httptest.NewServer(routes())
	defer srv.Close()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestHome(t *testing.T) {
	code, html := get(t, "/")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	for _, want := range []string{
		"<!DOCTYPE html>",
		"<title>Posts · gostro blog</title>",
		`<a href="/" class="nav-link active">Posts</a>`,
		"Layouts are components with slots",
		`<a class="more" href="/posts/slots">Read more →</a>`,
		"<footer><p>Total posts: 3</p></footer>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	// The tag style is registered once, even though the page has many tags.
	if n := strings.Count(html, ".tag--active {"); n != 1 {
		t.Errorf("tag CSS emitted %d times", n)
	}
	// Layout styles come first (base rules before component rules).
	if strings.Index(html, ":root {") > strings.Index(html, ".card {") {
		t.Error("style order")
	}
}

func TestTagFilter(t *testing.T) {
	_, html := get(t, "/?tag=security")
	if !strings.Contains(html, "Contextual escaping") || strings.Contains(html, "Hello, gostro") {
		t.Error("tag filter does not work")
	}
	if !strings.Contains(html, `class="tag tag--active"`) {
		t.Error("active tag is not highlighted")
	}
}

func TestPost(t *testing.T) {
	code, html := get(t, "/posts/escaping")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	if !strings.Contains(html, "The string &lt;script&gt;alert(1)&lt;/script&gt; in text") {
		t.Error("text is not escaped")
	}
	if !strings.Contains(html, "<script>\n  document.addEventListener") {
		t.Error("page script did not end up in {ctx.Scripts()}")
	}
}

func TestNotFound(t *testing.T) {
	for _, path := range []string{"/posts/nope", "/missing"} {
		code, html := get(t, path)
		if code != 404 || !strings.Contains(html, "<code>"+path+"</code>") {
			t.Errorf("%s: %d", path, code)
		}
	}
}
