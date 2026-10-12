; In Zed a later pattern overrides an earlier one, so general rules come first.

; ---------- Go ----------

(identifier) @variable
(type_identifier) @type
(field_identifier) @property
(package_identifier) @namespace

(parameter_declaration (identifier) @variable.parameter)
(variadic_parameter_declaration (identifier) @variable.parameter)

(label_name) @label

(const_spec name: (identifier) @constant)

((identifier) @variable.special
  (#match? @variable.special "^(props|slots|ctx)$"))

((identifier) @namespace
  (#eq? @namespace "__gostro"))

(call_expression
  function: (identifier) @function)

(call_expression
  function: (identifier) @function.builtin
  (#match? @function.builtin "^(append|cap|clear|close|complex|copy|delete|imag|len|make|max|min|new|panic|print|println|real|recover)$"))

(call_expression
  function: (selector_expression
    field: (field_identifier) @function.method))

(function_declaration
  name: (identifier) @function)

(method_declaration
  name: (field_identifier) @function.method)

(method_elem
  name: (field_identifier) @function.method)

((type_identifier) @type.builtin
  (#match? @type.builtin "^(any|bool|byte|comparable|complex64|complex128|error|float32|float64|int|int8|int16|int32|int64|rune|string|uint|uint8|uint16|uint32|uint64|uintptr)$"))

[
  "--"
  "-"
  "-="
  ":="
  "!"
  "!="
  "..."
  "*"
  "*="
  "/"
  "/="
  "&"
  "&&"
  "&="
  "&^"
  "&^="
  "%"
  "%="
  "^"
  "^="
  "+"
  "++"
  "+="
  "<-"
  "<<"
  "<<="
  "<="
  "="
  "=="
  ">="
  ">>"
  ">>="
  "|"
  "|="
  "||"
  "~"
] @operator

(binary_expression operator: ["<" ">"] @operator)

[
  "break"
  "case"
  "chan"
  "const"
  "continue"
  "default"
  "defer"
  "else"
  "fallthrough"
  "for"
  "func"
  "go"
  "goto"
  "if"
  "import"
  "interface"
  "map"
  "package"
  "range"
  "return"
  "select"
  "struct"
  "switch"
  "type"
  "var"
] @keyword

[
  (interpreted_string_literal)
  (raw_string_literal)
  (rune_literal)
] @string

(escape_sequence) @string.escape

[
  (int_literal)
  (float_literal)
  (imaginary_literal)
] @number

[
  (true)
  (false)
] @boolean

[
  (nil)
  (iota)
] @constant.builtin

["(" ")" "[" "]" "{" "}"] @punctuation.bracket
["," ";" "." ":"] @punctuation.delimiter

(comment) @comment

; ---------- Markup ----------

(tag_name) @tag
(tag_name (identifier) @tag)

; Component: <Card>, <ui.Card>
((tag_name (identifier) @type)
  (#match? @type "^[A-Z]"))
(tag_name (identifier) @namespace "." (identifier) @type)
(tag_name "." @punctuation.delimiter)

(attribute_name) @attribute
((attribute_name) @keyword
  (#match? @keyword "^(class:list|set:html|is:inline|slot)$"))

(attribute "=" @operator)
(quoted_attribute_value) @string

(start_tag ["<" ">"] @punctuation.bracket)
(end_tag ["</" ">"] @punctuation.bracket)
(self_closing_tag ["<" "/>"] @punctuation.bracket)

(interpolation ["{" "}"] @punctuation.special)

(doctype) @keyword
(html_comment) @comment
