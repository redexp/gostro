# Gostro for Zed

Syntax highlighting for gostro templates (`.gostro`). Zed does not support TextMate, so this is a dedicated tree-sitter grammar that extends [tree-sitter-go](https://github.com/tree-sitter/tree-sitter-go): a tag is just another Go expression, so the prefix-position rule (§3 of the spec) is resolved by the parser itself rather than by a heuristic. There is no LSP yet.

```
extension.toml           extension manifest
languages/gostro/        config.toml and queries: highlights, injections (CSS/JS/JSON), brackets, indents, outline
tree-sitter-gostro/      grammar (grammar.js → src/parser.c)
```

## Installation

Zed compiles the grammar itself, but only fetches it from a git repository at a specific commit. So set the commit SHA in `extension.toml` (`rev`); `repository` points at the GitHub repository, and for local development you can replace it with a `file://` URL pointing at your local checkout (commit your changes first).

Then in Zed: command palette → `zed: install dev extension` → select the `editors/zed` folder.

After changing `grammar.js` you need to regenerate `src/parser.c`, commit and update `rev`; after changing only `languages/gostro/*.scm`, the Rebuild button on the extension page (`zed: extensions`) is enough.

## Grammar development

```bash
npm install
```

```bash
npm run generate
```

```bash
npm run build:wasm
```

```bash
npm test
```

`npm test` parses every `.gostro` file in the repository plus the cases from the table in §3.4 of the spec, fails on any `ERROR`/`MISSING` node, and checks that the queries in `languages/gostro/` compile against the grammar. To print the tree of a single file: `node test/run.mjs --print ../../../examples/blog/home.gostro`.

`build:wasm` uses tree-sitter-cli 0.26, because it downloads wasi-sdk by itself and needs neither emcc nor docker; `generate` stays on 0.25 (ABI 15, which Zed supports).
