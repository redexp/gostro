// Checks the grammar: parses every .gostro in the repository plus the cases from
// gostro-spec.md §3.4 and fails if the tree contains ERROR/MISSING.
//
//   node test/run.mjs --wasm tree-sitter-gostro.wasm [--print file.gostro]

import { parseArgs } from 'node:util';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { Parser, Language, Query } from 'web-tree-sitter';

const here = dirname(fileURLToPath(import.meta.url));
const { values } = parseArgs({
  options: {
    wasm: { type: 'string', default: join(here, '..', 'tree-sitter-gostro.wasm') },
    print: { type: 'string' },
    root: { type: 'string', default: join(here, '..', '..', '..', '..') },
  },
});

await Parser.init();
const lang = await Language.load(resolve(values.wasm));
const parser = new Parser();
parser.setLanguage(lang);

if (values.print) {
  console.log(parser.parse(readFileSync(values.print, 'utf8')).rootNode.toString());
  process.exit(0);
}

let failed = 0;

function problems(tree) {
  const out = [];
  const walk = (n) => {
    if (n.isError || n.isMissing) out.push(`${n.isMissing ? 'MISSING ' + n.type : 'ERROR'} @ ${n.startPosition.row + 1}:${n.startPosition.column + 1}`);
    for (const c of n.children) walk(c);
  };
  walk(tree.rootNode);
  return out;
}

// Whole files from the repository.
function* gostroFiles(dir) {
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules' || name.startsWith('.')) continue;
    const p = join(dir, name);
    if (statSync(p).isDirectory()) yield* gostroFiles(p);
    else if (name.endsWith('.gostro') && !p.includes(join('testdata', 'errors'))) yield p;
  }
}

for (const file of gostroFiles(resolve(values.root))) {
  const errs = problems(parser.parse(readFileSync(file, 'utf8')));
  if (errs.length) {
    failed++;
    console.log(`FAIL ${file}\n  ${errs.join('\n  ')}`);
  } else {
    console.log(`ok   ${file}`);
  }
}

// §3.4: [input, expected number of tags]
const cases = [
  ['if x < y {\n}\n', 0],
  ['a := b << 2\n', 0],
  ['a <<= 2\n', 0],
  ['v := <-ch\n', 0],
  ['func f() <-chan int {\n\treturn nil\n}\n', 0],
  ['for i := 0; i < n; i++ {\n}\n', 0],
  ['x := <div/>\n', 1],
  ['func f() {\n\treturn <p>{t}</p>\n}\n', 1],
  ['xs := []Node{<li/>, <li/>}\n', 2],
  ['m := map[string]int{"a<b": 1}\n', 0],
  ['// <div>\n/* <div> */\n', 0],
  ['s := "<div>"\n', 0],
  ['s := `<div>`\n', 0],
  ['<select name="x"><option value="1">{"a"}</option></select>\n', 2],
  ['<div>\n  n := len(xs)\n  <span>{n}</span>\n</div>\n', 2],
  ['<p>{/* comment */}</p>\n', 1],
  ['<ul>\n  for _, x := range []int{1, 2} {\n    <li>{x}</li>\n  }\n</ul>\n', 2],
  ['<label for="x" class:list={[]any{"a", map[string]bool{"b": true}}}><input id="x" disabled></label>\n', 2],
];

const tagQuery = new Query(lang, '(element) @e');
for (const [src, want] of cases) {
  const tree = parser.parse(src);
  const errs = problems(tree);
  const got = tagQuery.captures(tree.rootNode).length;
  if (errs.length || got !== want) {
    failed++;
    console.log(`FAIL ${JSON.stringify(src)}: ${got} tags, expected ${want} ${errs.join(', ')}\n  ${tree.rootNode.toString()}`);
  } else {
    console.log(`ok   ${JSON.stringify(src)}`);
  }
}

// Zed queries must compile against this grammar.
const queriesDir = join(here, '..', '..', 'languages', 'gostro');
for (const name of readdirSync(queriesDir).filter((n) => n.endsWith('.scm'))) {
  try {
    new Query(lang, readFileSync(join(queriesDir, name), 'utf8'));
    console.log(`ok   query ${name}`);
  } catch (e) {
    failed++;
    console.log(`FAIL query ${name}: ${e.message}`);
  }
}

if (failed) {
  console.log(`\n${failed} failures`);
  process.exit(1);
}
console.log('\nall good');
