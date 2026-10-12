// Tokenizes .gostro with the same engine VS Code uses (vscode-textmate + oniguruma).
//
//   node test/tokenize.mjs --file ../../examples/blog/home.gostro   print tokens
//   node test/tokenize.mjs --check                                  all .gostro in the repository
//
// --check fails if a rule is left unclosed at the end of a file (e.g. embedded
// JS swallowed </script>) or no tag is recognized.
// CSS/JS grammars are taken from the installed VS Code (--vscode; by default
// it is looked up in %LOCALAPPDATA%\Programs\Microsoft VS Code).

import { parseArgs } from 'node:util';
import { readFileSync, readdirSync, statSync, existsSync } from 'node:fs';
import { join, dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createRequire } from 'node:module';
import vsctm from 'vscode-textmate';
import onig from 'vscode-oniguruma';

const here = dirname(fileURLToPath(import.meta.url));
const require = createRequire(import.meta.url);

const { values } = parseArgs({
  options: {
    file: { type: 'string' },
    check: { type: 'boolean', default: false },
    vscode: { type: 'string' },
    root: { type: 'string', default: join(here, '..', '..', '..') },
  },
});

function findVSCode() {
  const base = join(process.env.LOCALAPPDATA ?? '', 'Programs', 'Microsoft VS Code');
  for (const dir of [join(base, 'resources', 'app'), ...(existsSync(base) ? readdirSync(base).map((d) => join(base, d, 'resources', 'app')) : [])]) {
    if (existsSync(join(dir, 'extensions', 'css'))) return dir;
  }
  return null;
}

const vscode = values.vscode ?? findVSCode();
const files = {
  'source.gostro': join(here, '..', 'syntaxes', 'gostro.tmLanguage.json'),
  ...(vscode && {
    'source.css': join(vscode, 'extensions', 'css', 'syntaxes', 'css.tmLanguage.json'),
    'source.js': join(vscode, 'extensions', 'javascript', 'syntaxes', 'JavaScript.tmLanguage.json'),
  }),
};

await onig.loadWASM(readFileSync(require.resolve('vscode-oniguruma/release/onig.wasm')).buffer);
const registry = new vsctm.Registry({
  onigLib: Promise.resolve({
    createOnigScanner: (p) => new onig.OnigScanner(p),
    createOnigString: (s) => new onig.OnigString(s),
  }),
  loadGrammar: async (scope) => (files[scope] ? vsctm.parseRawGrammar(readFileSync(files[scope], 'utf8'), files[scope]) : null),
});
const grammar = await registry.loadGrammar('source.gostro');

function tokenize(text) {
  let stack = vsctm.INITIAL;
  const lines = [];
  for (const line of text.split(/\r?\n/)) {
    const r = grammar.tokenizeLine(line, stack);
    lines.push(r.tokens.map((t) => ({ text: line.slice(t.startIndex, t.endIndex), scopes: t.scopes })));
    stack = r.ruleStack;
  }
  return { lines, stack };
}

if (values.file) {
  for (const tokens of tokenize(readFileSync(values.file, 'utf8')).lines) {
    console.log(tokens
      .filter((t) => t.text.trim())
      .map((t) => `${JSON.stringify(t.text)}:${t.scopes.at(-1).replace(/\.gostro$/, '')}`)
      .join('  '));
  }
  process.exit(0);
}

if (!values.check) {
  console.log('--file or --check is required');
  process.exit(2);
}

function* gostroFiles(dir) {
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules' || name.startsWith('.')) continue;
    const p = join(dir, name);
    if (statSync(p).isDirectory()) yield* gostroFiles(p);
    else if (name.endsWith('.gostro') && !p.includes(join('testdata', 'errors'))) yield p;
  }
}

let failed = 0;
for (const file of gostroFiles(resolve(values.root))) {
  const text = readFileSync(file, 'utf8');
  const { lines, stack } = tokenize(text);
  const errs = [];
  if (stack.depth > 1) errs.push(`unclosed rule at end of file (depth ${stack.depth})`);
  const tags = lines.flat().filter((t) => t.scopes.some((s) => /^(entity\.name\.tag|support\.class\.component)\.gostro$/.test(s))).length;
  if (/<[A-Za-z]/.test(text) && tags === 0) errs.push('no tags recognized');
  if (errs.length) {
    failed++;
    console.log(`FAIL ${file}: ${errs.join('; ')}`);
  } else {
    console.log(`ok   ${file}`);
  }
}
if (!vscode) console.log('\n(VS Code CSS/JS grammars not found — embedded languages were not checked)');
process.exit(failed ? 1 : 0);
