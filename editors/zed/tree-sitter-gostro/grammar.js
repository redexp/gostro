/**
 * @file gostro: Go with markup literals
 * @license MIT
 *
 * Extends tree-sitter-go: a tag is an expression (the prefix-position rule from
 * gostro-spec.md §3 is resolved here by the LR parser itself — a binary `<` is only
 * possible after an expression, and a tag only where an expression is expected).
 */

/// <reference types="tree-sitter-cli/dsl" />
// @ts-check

const Go = require('tree-sitter-go/grammar');

const terminator = choice(/\n/, ';', '\0');

// gostro-spec.md §4.3
const VOID_ELEMENTS = [
  'area', 'base', 'br', 'col', 'embed', 'hr', 'img', 'input',
  'link', 'meta', 'param', 'source', 'track', 'wbr',
];

// HTML tags that coincide with Go keywords.
const KEYWORD_TAGS = ['select', 'map', 'var'];

module.exports = grammar(Go, {
  name: 'gostro',

  conflicts: ($, original) => original.concat([
    [$.element_children, $._expression],
  ]),

  rules: {
    _expression: ($, original) => choice($._markup, original),

    _markup: $ => choice($.element, $.doctype, $.html_comment),

    element: $ => choice(
      seq($.start_tag, optional($.element_children), $.end_tag),
      alias($.void_start_tag, $.start_tag),
      $.self_closing_tag,
      $.style_element,
      $.script_element,
    ),

    // Child position (§4.1): `{…}` is always an interpolation; there is no bare Go block.
    element_children: $ => repeat1(choice(
      $.interpolation,
      $._markup,
      seq($._child_statement, terminator),
      terminator,
    )),

    _child_statement: $ => choice(
      $._declaration,
      $._simple_statement,
      $.return_statement,
      $.go_statement,
      $.defer_statement,
      $.if_statement,
      $.for_statement,
      $.expression_switch_statement,
      $.type_switch_statement,
      $.select_statement,
      $.labeled_statement,
      $.fallthrough_statement,
      $.break_statement,
      $.continue_statement,
      $.goto_statement,
    ),

    start_tag: $ => seq(
      '<',
      field('name', $.tag_name),
      repeat($.attribute),
      '>',
    ),

    void_start_tag: $ => seq(
      '<',
      field('name', alias($._void_tag_name, $.tag_name)),
      repeat($.attribute),
      '>',
    ),

    self_closing_tag: $ => seq(
      '<',
      field('name', choice($.tag_name, alias($._void_tag_name, $.tag_name))),
      repeat($.attribute),
      '/>',
    ),

    end_tag: $ => seq(
      '</',
      field('name', $.tag_name),
      '>',
    ),

    tag_name: $ => choice(
      seq($.identifier, optional(seq('.', $.identifier))),
      ...KEYWORD_TAGS.map(k => alias(k, $.identifier)),
    ),

    _void_tag_name: _ => choice(...VOID_ELEMENTS),

    // <style>/<script>: content is taken verbatim up to the closing tag (§4.3, §7).
    style_element: $ => seq(
      alias($._style_start_tag, $.start_tag),
      optional($.raw_text),
      alias($._style_end_tag, $.end_tag),
    ),

    script_element: $ => seq(
      alias($._script_start_tag, $.start_tag),
      optional($.raw_text),
      alias($._script_end_tag, $.end_tag),
    ),

    _style_start_tag: $ => seq('<', field('name', alias('style', $.tag_name)), repeat($.attribute), '>'),
    _style_end_tag: $ => seq('</', field('name', alias('style', $.tag_name)), '>'),
    _script_start_tag: $ => seq('<', field('name', alias('script', $.tag_name)), repeat($.attribute), '>'),
    _script_end_tag: $ => seq('</', field('name', alias('script', $.tag_name)), '>'),

    raw_text: _ => token(prec(1, repeat1(choice(/[^<]/, /<[^/]/)))),

    attribute: $ => seq(
      field('name', $.attribute_name),
      optional(seq(
        '=',
        field('value', choice($.quoted_attribute_value, $.interpolation)),
      )),
    ),

    attribute_name: _ => /[a-zA-Z_@][a-zA-Z0-9_:.\-]*/,

    quoted_attribute_value: _ => token(choice(/"[^"]*"/, /'[^']*'/)),

    interpolation: $ => seq('{', optional($._expression), '}'),

    doctype: _ => token(seq('<!', /[dD][oO][cC][tT][yY][pP][eE]/, /[^>]*/, '>')),

    html_comment: _ => token(seq('<!--', /([^-]|-[^-]|--[^>])*/, '-->')),
  },
});
