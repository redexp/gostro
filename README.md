# gostro

[![Go Reference](https://pkg.go.dev/badge/github.com/redexp/gostro.svg)](https://pkg.go.dev/github.com/redexp/gostro)

An HTML template engine for Go: a superset of Go syntax with markup literals. `.gostro` files are transpiled into plain `.go` files, so props are type-checked by the Go compiler. The full design is described in [gostro-spec.md](gostro-spec.md).

```go
// card.gostro
import "strings"

type CardProps struct {
    Title string
    Tags  []string
    Href  string
}

slug := strings.ToLower(strings.ReplaceAll(props.Title, " ", "-"))

<article id={slug} class:list={[]any{"card", map[string]bool{"card--wide": len(props.Tags) > 3}}}>
  <a href={props.Href}><h2>{props.Title}</h2></a>
  if len(props.Tags) > 0 {
    <ul>
      for _, t := range props.Tags {
        <li class="tag">{t}</li>
      }
    </ul>
  }
  <slot>{"Empty"}</slot>
</article>

<style>
  .card { border: 1px solid #ddd; }
</style>
```

Running `gostro generate` produces `card_gostro.go` with `func Card(props CardProps, slots __gostro.Slots) __gostro.Node`.

## Install

Install the CLI:

```bash
go install github.com/redexp/gostro/cmd/gostro@latest
```

Add the runtime to your module (generated code imports `github.com/redexp/gostro/runtime`):

```bash
go get github.com/redexp/gostro
```

Generate code for every `.gostro` file in the module:

```bash
gostro generate ./...
```

Generated files are meant to be committed, so users of your package can `go get` it without installing the CLI. `//go:generate gostro generate` is supported.

## Quick start (this repository)

```bash
go test ./...
```

```bash
go run ./examples/blog
```

```bash
go run ./cmd/gostro watch -cmd "go run ./examples/blog" ./examples/blog
```

`watch` regenerates code on change, restarts the app and reloads the page in the browser over SSE. Put `{__gostro.LiveReload()}` in your layout; outside `watch` it renders nothing.

## Syntax at a glance

| What | How |
|---|---|
| Text | only `{"…"}`; bare text between tags is a compile error with a hint |
| Expression | `{props.Title}`: strings are escaped, `Node`/`[]Node` are rendered, `RawHTML` is written as is |
| Attributes | `a="literal"`, `a={expr}`, `a`, `class:list={[]any{…}}`, `set:html={…}` |
| Control flow | plain Go `if`/`else`, `for`, `switch`, `select` with tags inside |
| Statements in markup | `n := len(xs)` right among the children |
| Tags as values | `items := []__gostro.Node{<li>{"a"}</li>}`, `return <p/>` |
| Props | `type CardProps struct{…}` in `card.gostro`, under the same name as in the generated code; every top-level `type XxxProps struct` is hoisted to package level |
| Components | `<Card title="x" max-width={10}/>`, `<ui.Card>…</ui.Card>`; `max-width` maps to the `MaxWidth` field |
| Slots | `<slot/>`, `<slot name="head">{"fallback"}</slot>`; at the call site: `<h2 slot="head">` |
| Styles/scripts | `<style>`/`<script>` are registered in `ctx` once and emitted by `{ctx.Styles()}`/`{ctx.Scripts()}`; `is:inline` keeps them in place |
| JSON in scripts | `<script>const d = {__gostro.JSON(v)}</script>` |
| Comments | `{/* … */}`, `// …`, `<!-- … -->` (the last one is kept in the output) |

Escaping is contextual: text, attribute values, URLs (`javascript:`, `vbscript:` and `data:` other than `data:image/*` are replaced with `about:invalid#gostro`) and JSON inside `<script>`. Dynamic `on*` attributes and `{expr}` inside `<style>` are compile errors. Escaping can only be bypassed explicitly, with the `__gostro.RawHTML` and `__gostro.SafeURL` types or the `set:html` directive.

Go compiler errors point to the line and column in the `.gostro` file, thanks to `//line` and `/*line*/` directives in the generated code.

## net/http

```go
mux.Handle("GET /{$}", __gostro.Handler(func(r *http.Request) __gostro.Node {
    return Home(HomeProps{Posts: posts}, nil)
}))
```

`Handler` buffers the response (a failed render becomes a 500 instead of truncated HTML) and inserts the collected styles and scripts. Templates can use `ctx.Request()` and `ctx.SetStatus(404)`. Outside HTTP, use `__gostro.RenderTo` and `__gostro.RenderString`.

## Layout

```
cmd/gostro/       CLI: generate, watch, version
internal/token/   tokens, semicolon insertion, the prefix-position rule
internal/scan/    lexer
internal/ast/     AST nodes
internal/parse/   parser (Go code as opaque spans + markup)
internal/gen/     code generator
internal/e2e/     end-to-end tests: .gostro → Go → HTML
runtime/          public runtime (imported as __gostro)
testdata/golden/  *.gostro + expected *.go.want
testdata/errors/  *.gostro + expected error text *.err
examples/blog/    demo site
editors/vscode/   VS Code extension (TextMate grammar)
```

Update golden files:

```bash
go test ./internal/gen ./internal/e2e -update
```

## Differences from the spec and additions

- **Child interpolation** generates `__gostro.Child(ctx, w, v)` instead of `__gostro.Text(w, v)`: without `ctx`, `Text(w, v)` cannot render a `Node`, which table 4.5 requires. `Text` remains for scalars.
- **The writer** is named `__w` in generated code, not `w`, so it doesn't clash with a user variable `w`. `ctx`, `props` and `slots` keep their names.
- **Writer.SetErr**: the `Writer` interface also has `SetErr(error)`, because runtime errors (such as an unsupported type) are stored inside the writer. Generated code doesn't check `err` after every call and ends with `return __w.Err()`.
- **Styles and scripts via a marker.** `{ctx.Styles()}` in `<head>` renders before components get a chance to register their styles, so it writes a marker that `Handler`/`RenderTo` replace with the collected styles after rendering.
- **Hoisted registrations.** Unconditional `ctx.UseStyle`/`UseScript` calls are moved to the top of the component body, as in example 9.1, so layout styles come before the styles of nested components.
- **The component body is a list of children**, so a top-level `{expr}` is an interpolation too (section 4.1). In functions without tags, `{` is still a regular Go block.
- **`//line` columns.** For statements that start at column 1 in the `.gostro` file, the reported column is shifted by the indentation depth (as in example 9.1). Positions of expressions, attributes and component calls are exact.
- **Runtime additions**: `Child`, `BoolAttr`, `Fragment`, `RenderTo`, `RenderString`, `Ctx.SetStatus`, `Ctx.Set/Get`, `LiveReload`.

## Not done yet

Stage 9 of the spec: `gostro fmt` (the command currently reports that it is not implemented) and an LSP proxy to `gopls`.

## License

[MIT](LICENSE)
