# Gostro for VS Code

Syntax highlighting for gostro templates (`.gostro`): Go code, tags and components, attributes and `{expressions}`, the `class:list`/`set:html`/`is:inline` directives, embedded CSS in `<style>` and JS in `<script>`. There is no LSP yet.

## Installation

```bash
npm install
```

```bash
npm run package
```

```bash
code --install-extension gostro-0.1.0.vsix
```

For development, open this folder in VS Code and press F5: an Extension Development Host window starts with the extension loaded.

## Checking the grammar

```bash
npm test
```

The script tokenizes every `.gostro` file in the repository with the same engine VS Code uses, and fails if a rule is left unclosed at the end of a file (a typical sign that embedded JS/CSS "swallowed" the closing tag). To print the tokens of a single file: `node test/tokenize.mjs --file ../../examples/blog/home.gostro`.

## Limitations

TextMate works with line-based regular expressions and does not see a syntax tree, so the prefix-position rule is approximated: `<` is treated as the start of a tag if it is immediately followed by a name and is not preceded by an identifier, `)` or `]`. An expression like `if a <b {` (with no space after `<`) will be wrongly highlighted as a tag.
