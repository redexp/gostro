package main

import (
	"sort"
	"time"
)

// Post is a blog post.
type Post struct {
	Slug    string
	Title   string
	Date    time.Time
	Tags    []string
	Summary string
	Body    []string // paragraphs
}

var posts = []Post{
	{
		Slug:    "hello-gostro",
		Title:   "Hello, gostro",
		Date:    time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		Tags:    []string{"go", "templates"},
		Summary: "Templates as plain Go code with tags inside.",
		Body: []string{
			"gostro transpiles .gostro files into plain Go: the compiler checks prop types.",
			"All text is written inside {\"…\"}, so whitespace and line breaks are always explicit.",
		},
	},
	{
		Slug:    "escaping",
		Title:   "Contextual escaping",
		Date:    time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		Tags:    []string{"security"},
		Summary: "Text, attributes and URLs are escaped differently — and the compiler decides how.",
		Body: []string{
			"The string <script>alert(1)</script> in text becomes safe.",
			"javascript:… links are replaced with about:invalid#gostro.",
		},
	},
	{
		Slug:    "slots",
		Title:   "Layouts are components with slots",
		Date:    time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC),
		Tags:    []string{"go", "components", "slots"},
		Summary: "Named slots, fallbacks and no special entities for layouts.",
		Body: []string{
			"Children without a slot attribute go into the default slot.",
		},
	},
}

func postsByTag(tag string) []Post {
	var out []Post
	for _, p := range posts {
		if tag == "" || contains(p.Tags, tag) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out
}

func findPost(slug string) (Post, bool) {
	for _, p := range posts {
		if p.Slug == slug {
			return p, true
		}
	}
	return Post{}, false
}

func allTags() []string {
	seen := map[string]bool{}
	var tags []string
	for _, p := range posts {
		for _, t := range p.Tags {
			if !seen[t] {
				seen[t] = true
				tags = append(tags, t)
			}
		}
	}
	sort.Strings(tags)
	return tags
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
