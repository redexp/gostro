((comment) @injection.content
  (#set! injection.language "comment"))

((style_element (raw_text) @injection.content)
  (#set! injection.language "css"))

((script_element
  (start_tag
    (attribute
      name: (attribute_name) @_name
      value: (quoted_attribute_value) @_type))
  (raw_text) @injection.content)
  (#eq? @_name "type")
  (#match? @_type "json")
  (#set! injection.language "json"))

((script_element
  (start_tag) @_start
  (raw_text) @injection.content)
  (#not-match? @_start "type=[\"'][^\"']*json")
  (#set! injection.language "javascript"))
