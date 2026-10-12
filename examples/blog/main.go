// A gostro demo site: a layout, components with slots, net/http routing.
//
//	go run ./examples/blog                     # http://localhost:8080
//	go run ./cmd/gostro watch -cmd "go run ./examples/blog" ./examples/blog
package main

//go:generate go run github.com/redexp/gostro/cmd/gostro generate .

import (
	"flag"
	"log"
	"net/http"

	__gostro "github.com/redexp/gostro/runtime"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP server address")
	flag.Parse()
	log.Printf("blog: http://localhost%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, routes()))
}

func routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /{$}", __gostro.Handler(func(r *http.Request) __gostro.Node {
		tag := r.URL.Query().Get("tag")
		return Home(HomeProps{Posts: postsByTag(tag), Tag: tag, Tags: allTags()}, nil)
	}))
	mux.Handle("GET /posts/{slug}", __gostro.Handler(func(r *http.Request) __gostro.Node {
		p, ok := findPost(r.PathValue("slug"))
		if !ok {
			return NotFound(NotFoundProps{Path: r.URL.Path}, nil)
		}
		return PostPage(PostPageProps{Post: p}, nil)
	}))
	mux.Handle("GET /about", __gostro.Handler(func(r *http.Request) __gostro.Node {
		return About(AboutProps{}, nil)
	}))
	mux.Handle("/", __gostro.Handler(func(r *http.Request) __gostro.Node {
		return NotFound(NotFoundProps{Path: r.URL.Path}, nil)
	}))
	return mux
}
