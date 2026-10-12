# gostro — specification

An HTML template engine for Go: a superset of Go syntax with Astro/JSX-style markup literals. `.gostro` files are transpiled into plain `.go` files.

This document is the complete technical specification. Read it in full before writing any code.

---

## 0. Key decisions (do not change without explicit approval)

1. **Code-first.** A file is Go code with tags embedded in it. No frontmatter, no `---`.
2. **Code generation, not interpretation.** The output is a `.go` file that `go build` compiles. Prop types are checked by the Go compiler.
3. **All text lives in `{"…"}` only.** Bare text content between tags does not exist. This eliminates every question about whitespace, ASI conflicts and reserved words.
4. **No scoped CSS.** No hashes, no `data-*` attributes in the output. The markup comes out exactly as written.
5. **The extension is `.gostro`.** Generated file: `card.gostro` → `card_gostro.go`.

---

## 1. Example input file

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

---

## 2. File structure

A `.gostro` file is the **implicit body of a component**. Layout during generation:

| Construct in the input file | Where it ends up |
|---|---|
| `package x` (optional, first line only) | header of the generated file |
| `import` (single or block) | import block of the generated file |
| top-level `func …` | package level |
| `type XxxProps struct{…}` (name ends in `Props`) | package level, verbatim |
| everything else (`type`, `const`, `var`, assignments, expressions, tags) | body of the component function, in source order |

The **component name** is derived from the file name: `card.gostro` → `Card`, `blog-post.gostro` → `BlogPost`, `user_card.gostro` → `UserCard`.

The **package name** is determined as follows, in decreasing order of priority: an explicit `package` directive in the file → the package of sibling `.go` files in the same directory → the directory name, normalized to a Go identifier.

### 2.1 Props type

The user declares the props type themselves, **under the name it will have in the generated code**: `<ComponentName>Props`. No renaming and no aliases — the type is the same in `.gostro` and in `.go`.

```go
// card.gostro
type CardProps struct {
    Title string
}

<h2>{props.Title}</h2>
```

→

```go
type CardProps struct {
    Title string
}

func Card(props CardProps, slots __gostro.Slots) __gostro.Node {
    return __gostro.NodeFunc(func(ctx *__gostro.Ctx, w __gostro.Writer) error {
        ...
    })
}
```

Hoisting rule: any top-level `type XxxProps struct{…}` (name longer than `Props` and ending in `Props`) is moved to package level verbatim. Props of explicit components (`BadgeProps` in 2.2) are declared the same way. `type Props` is nothing special: like any other type, it becomes a local type in the component body.

If `<ComponentName>Props` is not declared in the file, an empty struct `<Name>Props struct{}` is generated — for components without props. Consequently, this type must not be declared in a separate `.go` file: that would cause a name conflict.

The implicit component is generated if the file has an implicit body or declares `<ComponentName>Props` (and there is no explicit function with the same name).

### 2.2 Multiple components in one file

There is one rule, with no exceptions: **a function whose body contains a tag is a component.** The generator rewrites its signature to return `__gostro.Node`.

```go
type BadgeProps struct {
    Text string
}

func Badge(props BadgeProps) {
    <span class="badge">{props.Text}</span>
}
```

→

```go
type BadgeProps struct {
    Text string
}

func Badge(props BadgeProps, slots __gostro.Slots) __gostro.Node {
    return __gostro.NodeFunc(func(ctx *__gostro.Ctx, w __gostro.Writer) error { … })
}
```

If the function has exactly one parameter of type `<Name>Props`, the generator adds a `slots __gostro.Slots` parameter so it can be invoked as `<Badge …/>` (section 5.4). The implicit file body is just sugar over this form. If a file has both an implicit body and explicit component functions, both work.

---

## 3. Lexer: the prefix position rule

This is the foundation of everything. Build it first and cover it with tests before moving on.

### 3.1 The rule

In Go, `<` **does not exist as a unary operator**. In a position where the start of an expression is expected (prefix position), `<` cannot be a comparison. The only legal construct there is `<-` (channel).

> **`<` in prefix position, not followed by `-`, starts a tag. In any other position it is `<`, `<=` or `<<`.**

Generics cause no problems: Go uses `[]`, not `<>`. The grammar extension is formally unambiguous.

### 3.2 Defining prefix position

A position is prefix if the preceding **significant** token (not whitespace, not a comment) is one of:

- start of file
- `{` `}` `(` `[` `,` `;` (including an automatically inserted semicolon)
- `=` `:=` `+=` `-=` `*=` `/=` `%=` `&=` `|=` `^=` `<<=` `>>=` `&^=`
- any binary or unary operator: `+` `-` `*` `/` `%` `&` `|` `^` `<<` `>>` `&&` `||` `!` `==` `!=` `<` `<=` `>` `>=` `<-`
- the keywords `return`, `case`, `default`, `go`, `defer`, `range`, `chan`, `else`
- `:` (a label or the end of a `case`)

A position is **infix** after: an identifier, a literal (number, string, rune), `)`, `]`, `}`, the keywords `true`/`false`/`nil` (they are identifiers).

### 3.3 ASI

Automatic semicolon insertion must be implemented following Go's rules: `;` is inserted at the end of a line if the last token is an identifier, a literal, or one of `break continue fallthrough return ++ -- ) ] }`.

### 3.4 Required lexer tests

| Input | Expected |
|---|---|
| `if x < y {` | operator `<` |
| `a := b << 2` | operator `<<` |
| `a <<= 2` | operator `<<=` |
| `v := <-ch` | channel receive |
| `func f() <-chan int` | channel type |
| `for i := 0; i < n; i++ {` | operator `<` |
| `x := <div/>` | **tag** |
| `return <p>{t}</p>` | **tag** |
| `xs := []Node{<li/>, <li/>}` | **two tags** |
| `m := map[string]int{"a<b": 1}` | string, not a tag |
| `// <div>` and `/* <div> */` | comment, not a tag |
| `s := "<div>"` | string, not a tag |
| `` s := `<div>` `` | raw string, not a tag |

The generator **does not fully parse Go**. It only needs a lexer with ASI and position tracking; the rest of the Go code is treated as opaque text spans passed through to the output verbatim.

---

## 4. Markup grammar

### 4.1 Child position

Inside an element, dispatch is based on the first significant token:

| Token | Interpretation |
|---|---|
| `<` | nested element or component |
| `{` | interpolation `{expr}` |
| `if` / `for` / `switch` / `select` | control construct with child bodies |
| anything else | Go statement, passed through verbatim up to the end of the statement (ASI or `;`) |

The last branch allows local declarations directly inside markup:

```go
<div>
  n := len(props.Tags)
  <span>{n}</span>
</div>
```

**Bare text does not exist.** `<p>Hello</p>` is a syntax error with the hint `text must be inside {"…"}: <p>{"Hello"}</p>`.

**`{` in child position always opens an interpolation**, never a bare Go block. If you need a block, use `if true { … }`.

### 4.2 Control constructs

The generator **does not understand them semantically**. It finds the clause, passes it through verbatim, and parses the body recursively as children.

Algorithm for `if`/`for`:
1. Read tokens from the keyword up to the `{` that opens the block. Track nesting of `()`, `[]`, `{}` — composite literals in the clause (`for _, x := range []int{1,2}`) must not stop the scan.
2. Emit the clause verbatim.
3. Recursively parse children up to the matching `}`.
4. For `if`: if the next significant token is `else`, continue (`else if …` or `else { … }`).

`switch` and `select` are handled separately: their body is expected to contain `case X:` / `default:` clauses, each followed by children up to the next clause or the end of the block.

### 4.3 Elements

- **Void elements** (no closing tag, hard-coded list): `area base br col embed hr img input link meta param source track wbr`.
- **Self-closing** `<div/>` is allowed for any element.
- **Raw-text elements**: `<script>` and `<style>` — their content is taken verbatim up to the closing tag, without parsing tags or `{}`.
- Other elements (`<pre>`, `<textarea>`) need no special handling: since all text is in `{"…"}`, whitespace and line breaks are specified explicitly in the string literal.
- An unclosed or mismatched tag is a compile error reporting the positions of both tags.

**Do not use `golang.org/x/net/html`.** It normalizes the tree (inserts `<tbody>`, moves nodes) and knows nothing about `{}` or `<Card />`. A custom parser is required; it is about 600 lines.

### 4.4 Attributes

| Form | Semantics |
|---|---|
| `name="literal"` | static value |
| `name={expr}` | dynamic value |
| `name` | boolean attribute, always present |
| `class:list={expr}` | directive, see below |
| `set:html={expr}` | element content as raw HTML; children are forbidden |

**Boolean attributes** (`disabled checked readonly required selected multiple autofocus hidden open`) with a dynamic value of type `bool` are rendered conditionally: `disabled={cond}` → the attribute is present only if `cond`.

**`class:list`** takes `[]any` and joins the elements: a `string` is added as is, `[]string` element by element, `map[string]bool` contributes keys with true values, `nil` and empty strings are skipped. The order is deterministic: for maps, sorted by key.

**Interpolation inside an attribute value** is only partially allowed: `class="btn btn-{size}"` is not supported in v1. Only a whole value: `class={expr}`. For concatenation, use a Go expression.

### 4.5 Interpolation `{expr}`

Scanning for balanced `{}` must account for: strings `"…"`, raw strings `` `…` ``, runes `'…'`, and comments `//` and `/* */`. Without this, a `map[string]bool{…}` inside an attribute breaks the parser.

The value inside `{}` is dispatched at runtime via a type switch:

| Type | Behavior |
|---|---|
| `string` | escaped according to context |
| `__gostro.Node` | rendered recursively |
| `__gostro.RawHTML` | written without escaping |
| `__gostro.SafeURL` | URL without scheme checking |
| numeric types, `bool` | formatted and escaped |
| `fmt.Stringer` | `String()`, then as `string` |
| `error` | `Error()`, then as `string` |
| `nil` | nothing is written |
| anything else | runtime error describing the type |

### 4.6 Tags in expression position

A tag in prefix position is a **value of type `__gostro.Node`**. It can be put in slices and maps, returned from functions, and passed in props:

```go
items := []__gostro.Node{
    <li>{"one"}</li>,
    <li>{"two"}</li>,
}
<ul>{items}</ul>
```

It is generated as `__gostro.NodeFunc(func(ctx, w) error { … })`.

---

## 5. Components and slots

### 5.1 Recognition

A tag is a component call if its name starts with an uppercase letter (`<Card>`) or contains a dot (`<ui.Card>`). Resolution goes through ordinary Go imports, with no custom mechanism.

### 5.2 Props mapping

Attribute name → field of the `<ComponentName>Props` struct: split on `-`, capitalize each segment.

| Attribute | Field |
|---|---|
| `title` | `Title` |
| `href` | `Href` |
| `max-width` | `MaxWidth` |
| `aria-label` | `AriaLabel` |

An unknown field → a Go compile error (struct literal with a nonexistent field); thanks to `//line`, the position points at the attribute.

### 5.3 Slots

Definition inside a component:

```go
<slot>{"Empty"}</slot>                  // default slot with a fallback
<slot name="header"/>                   // named slot
```

Passing from the call site:

```go
<ui.Card title="x">
  <h2 slot="header">{"Title"}</h2>
  <p>{"Body"}</p>
</ui.Card>
```

Children without a `slot` attribute go into `"default"`. Layouts are not a separate entity — they are ordinary components with slots.

### 5.4 Call generation

```go
__gostro.Render(ctx, w, ui.Card(ui.CardProps{Title: "x"}, __gostro.Slots{
    "default": __gostro.NodeFunc(func(ctx *__gostro.Ctx, w __gostro.Writer) error { … }),
    "header":  __gostro.NodeFunc(func(ctx *__gostro.Ctx, w __gostro.Writer) error { … }),
}))
```

The `<ui.Card/>` form is sugar over the direct call `{ui.Card(ui.CardProps{…}, nil)}`. Both forms must work.

---

## 6. Escaping

HTML is parsed at compile time, so the context of every insertion is known and **exactly the right** escaper is generated rather than a universal one. Context violations are caught at `go build`, not at runtime — strictly stronger than `html/template`.

| Context | Call | Note |
|---|---|---|
| child text | `__gostro.Text(w, v)` | `& < > ' "` |
| attribute value | `__gostro.Attr(w, v)` | always in double quotes |
| `href` `src` `action` `formaction` `poster` `cite` | `__gostro.URL(w, v)` | block `javascript:`, `data:` (except `data:image/*`), `vbscript:` |
| inside `<script>` | only `__gostro.JSON(v)` | otherwise a **compile** error |
| inside `<style>` | forbidden | compile error |
| tag or attribute name | forbidden | compile error |

Opting out is possible only via the types `__gostro.RawHTML(s)`, `__gostro.SafeURL(s)` and the `set:html` directive. No "disable escaping" flags.

---

## 7. `<style>` and `<script>` without scoping

Scoped CSS is **not implemented**. A `<style>` block inside a component is:

1. Extracted from the markup at compile time (nothing is emitted in its place).
2. Registered in `*__gostro.Ctx` via `ctx.UseStyle(key, css)`, where `key` is the file path plus the block's ordinal number. Deduplicated by key: a component rendered 100 times adds its CSS once.
3. The page layout emits the collected styles via `{ctx.Styles()}` in `<head>`.

`<style is:inline>` is left in place verbatim, without registration.

`<script>` works the same way: `ctx.UseScript(key, js)` and `{ctx.Scripts()}`; `<script is:inline>` stays in place. `src`, `type="application/json"`, and any `<script>` with attributes are always inline.

CSS selectors are not rewritten, and no attributes are added to elements. The output is exactly as written.

---

## 8. Runtime (`runtime/`)

A public package that generated code depends on. It is imported under the alias `__gostro` to guarantee no conflicts with user names.

```go
package runtime

// Writer accumulates the first error internally.
// This lets the generator emit chains of WriteString calls without checking err on every line.
type Writer interface {
    WriteString(string)
    Write([]byte)
    Err() error
}

func NewWriter(w io.Writer) Writer

type Node interface {
    Render(ctx *Ctx, w Writer) error
}

type NodeFunc func(*Ctx, Writer) error
func (f NodeFunc) Render(ctx *Ctx, w Writer) error { return f(ctx, w) }

type Slots map[string]Node

func (s Slots) RenderOr(ctx *Ctx, w Writer, name string, fallback Node) error

type Ctx struct { /* style and script registry, context.Context, arbitrary values */ }

func NewCtx(ctx context.Context) *Ctx
func (c *Ctx) UseStyle(key, css string)
func (c *Ctx) UseScript(key, js string)
func (c *Ctx) Styles() Node
func (c *Ctx) Scripts() Node

type RawHTML string
type SafeURL string

func Text(w Writer, v any)
func Attr(w Writer, v any)
func URL(w Writer, v any)
func JSON(v any) RawHTML
func ClassList(w Writer, v []any)
func Render(ctx *Ctx, w Writer, n Node) error

// net/http integration
func Handler(build func(*http.Request) Node) http.Handler
```

The `Writer` with an internal error is a deliberate choice: it removes `if err != nil` after each of hundreds of write calls and keeps the generated code readable.

---

## 9. Code generation

### 9.1 Example output

For the file from section 1:

```go
// Code generated by gostro. DO NOT EDIT.

package components

import (
	"strings"

	__gostro "github.com/redexp/gostro/runtime"
)

type CardProps struct {
	Title string
	Tags  []string
	Href  string
}

func Card(props CardProps, slots __gostro.Slots) __gostro.Node {
	return __gostro.NodeFunc(func(ctx *__gostro.Ctx, w __gostro.Writer) error {
//line card.gostro:9:1
		slug := strings.ToLower(strings.ReplaceAll(props.Title, " ", "-"))

		ctx.UseStyle("card.gostro#0", "\n  .card { border: 1px solid #ddd; }\n")

		w.WriteString(`<article id="`)
		__gostro.Attr(w, slug)
		w.WriteString(`" class="`)
		__gostro.ClassList(w, []any{"card", map[string]bool{"card--wide": len(props.Tags) > 3}})
		w.WriteString(`"><a href="`)
		__gostro.URL(w, props.Href)
		w.WriteString(`"><h2>`)
		__gostro.Text(w, props.Title)
		w.WriteString(`</h2></a>`)

//line card.gostro:16:3
		if len(props.Tags) > 0 {
			w.WriteString(`<ul>`)
//line card.gostro:18:7
			for _, t := range props.Tags {
				w.WriteString(`<li class="tag">`)
				__gostro.Text(w, t)
				w.WriteString(`</li>`)
			}
			w.WriteString(`</ul>`)
		}

		if err := slots.RenderOr(ctx, w, "default",
			__gostro.NodeFunc(func(ctx *__gostro.Ctx, w __gostro.Writer) error {
				w.WriteString(`Empty`)
				return nil
			})); err != nil {
			return err
		}

		w.WriteString(`</article>`)
		return w.Err()
	})
}
```

Note that `if` and `for` are copied one-to-one. The generator does not analyze them.

### 9.2 `//line` directives

**Required from day one.** Before every passed-through Go fragment:

```
//line card.gostro:18:7
```

The directive must be on its own line and applies to the following line. It redirects compiler errors, `go vet` output and panic stack traces to the real `.gostro` file. It replaces roughly half of the diagnostics work — without it, development becomes painful.

### 9.3 Optimizations

- **Static text merging.** Adjacent static fragments are merged into a single `WriteString` with one long literal. In the example above, `</h2></a>` is the result of merging three nodes.
- **Backtick literals** for static content if it contains no `` ` `` and no non-printable characters; otherwise a regular escaped string.
- Static attribute values are escaped at compile time and embedded in the literal, with no runtime call.
- Whitespace and line breaks between markup nodes are **removed entirely** — they cannot carry meaning, since all text is explicit. If you need a space, use `{" "}`.

### 9.4 Determinism

The same input always produces byte-for-byte identical output. No maps with nondeterministic iteration, no timestamps in the output file.

---

## 10. CLI

```
gostro generate [paths...]     # transpile (default ./...)
gostro fmt [paths...]          # format .gostro files
gostro watch [paths...]        # fsnotify + regeneration + SSE reload
gostro version
```

`generate` flags: `-w` (watch), `-v` (verbose), `-keep-going` (do not stop at the first error).

Support for `//go:generate gostro generate`.

**Generated files are committed to the repository.** That way `go get` of your library works without installing the CLI.

---

## 11. Errors

The format matches Go's, so editors recognize it:

```
card.gostro:12:5: bare text is not allowed; wrap it in {"…"}
card.gostro:18:3: tag <ul> is not closed (opened at 18:3)
card.gostro:24:9: tag </div> closes <span> opened at 22:5
card.gostro:31:7: {expr} inside <script> is not allowed; use {__gostro.JSON(v)}
```

Every error includes the file, line, column and the action to take. For the most common errors (bare text, unclosed tag), show the source line with an underline.

---

## 12. Project structure

```
gostro/
  cmd/gostro/           CLI
  internal/token/       tokens + ASI + prefix position rule
  internal/scan/        lexer (Go tokens + markup)
  internal/ast/         AST nodes
  internal/parse/       parser
  internal/gen/         code generator
  internal/fmtx/        formatter (late stage)
  runtime/              public runtime
  testdata/
    golden/             *.gostro + expected *.go.want
    errors/             *.gostro + expected error text
  examples/
    blog/               demo site with a layout, components, routing
```

AST:

```go
type Node interface{ Pos() Pos }

type GoChunk      struct{ Code string; Pos Pos }                    // Go passed through verbatim
type Element      struct{ Name string; Attrs []Attr; Children []Node; SelfClose bool }
type Component    struct{ Pkg, Name string; Attrs []Attr; Slots map[string][]Node }
type Interp       struct{ Code string; Ctx EscapeCtx }
type Control      struct{ Clause string; Body []Node; Else []Node }  // if / for
type Switch       struct{ Clause string; Cases []SwitchCase }
type SlotDef      struct{ Name string; Fallback []Node }
type StyleBlock   struct{ CSS string; Inline bool; Key string }
type ScriptBlock  struct{ JS string; Inline bool; Key string }
```

---

## 13. Work plan

Proceed strictly in order. Each stage must work and have tests before moving on to the next.

**Stage 1 — lexer.** `internal/token` + `internal/scan`. Go tokens, ASI, prefix position tracking. All tests from table 3.4 must pass. This is the riskiest part; do not move on with red tests.

**Stage 2 — markup parser.** Elements, attributes, void and raw-text tags, `{expr}` with brace balancing, an error on bare text. The AST is built; no codegen yet.

**Stage 3 — basic codegen.** Static content, `{expr}` with contextual escaping, `//line`, static text merging, imports, function signature, hoisting of `type XxxProps`. Golden tests.

**Stage 4 — control constructs.** Passing through `if`/`for`/`switch`/`select` with children inside. Passing through arbitrary Go statements in child position. **After this stage the engine is already usable.**

**Stage 5 — tags in expression position.** `__gostro.Node` as a value, tags in slices and as arguments.

**Stage 6 — components and slots.** Props mapping, named slots, fallbacks.

**Stage 7 — style/script.** Extraction, registration in `Ctx`, deduplication, `is:inline`.

**Stage 8 — CLI and DX.** `generate`, `watch` on `fsnotify`, SSE reload, `net/http` integration, the `examples/blog` example.

**Stage 9 — formatter and LSP proxy.** Only once the syntax has stabilized. LSP proxy: maintain a full `.gostro` ↔ `.go` position map, translate editor requests into `.go` coordinates, query the real `gopls`, translate the response back. Autocompletion, go-to-definition and type errors then work almost completely.

---

## 14. Testing

- **Golden tests** — the primary mechanism. `testdata/golden/x.gostro` + `x.go.want`, compared after `gofmt`. An `-update` flag to rewrite them.
- **Error tests** — `testdata/errors/x.gostro` + `x.err`, comparing the error text including position.
- **Compilation tests** — every golden output must actually compile: `go build` in a temporary directory.
- **Runtime tests** — render a component into a buffer and compare the HTML.
- **Fuzzing** the lexer: it must not panic on any input.
- **Security tests** — a separate suite: XSS vectors in text, attributes, URLs (`javascript:`, `JaVaScRiPt:`, spaces and tabs inside the scheme, `data:text/html`).

---

## 15. What not to do

- Do not use `x/net/html` to parse templates.
- Do not go back to frontmatter or to block forms like `{if}…{/if}` — control constructs are plain Go.
- Do not implement scoped CSS, hashes or `data-*` attributes.
- Do not allow bare text between tags.
- Do not interpret templates at runtime.
- No islands or JS bundling in v1 — for Go projects, htmx or Alpine is usually enough.
- Do not add dependencies beyond `fsnotify` (watch) and `tdewolff/parse` (CSS/JS minification, optional).

---

## 16. References

- **`a-h/templ`** — the closest analogue architecturally: codegen, `//line`, contextual escaping, LSP proxy. Worth reading `generator/` and `lspcmd/` as a reference for the approach, but do not copy the syntax — ours is different.
- **`valyala/quicktemplate`** — ideas for output optimization.
- **The Astro compiler** (`withastro/compiler`) — partly written in Go; its parser is a useful reference for behavior in HTML edge cases.
- **`html/template`** — the source for lists of escaping contexts and dangerous URL schemes.
