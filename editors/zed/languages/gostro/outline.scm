(type_declaration
  (type_spec
    name: (type_identifier) @name)) @item

(function_declaration
  "func" @context
  name: (identifier) @name) @item

(method_declaration
  "func" @context
  receiver: (parameter_list) @context
  name: (field_identifier) @name) @item
