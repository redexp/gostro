// Package names implements naming rules: component name from file name, props field from attribute.
package names

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Component derives a component name from a file name:
// card.gostro → Card, blog-post.gostro → BlogPost, user_card.gostro → UserCard.
func Component(file string) string {
	base := file
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".gostro")
	var b strings.Builder
	for _, seg := range strings.FieldsFunc(base, func(r rune) bool { return r == '-' || r == '_' || r == '.' || r == ' ' }) {
		b.WriteString(upperFirst(seg))
	}
	s := b.String()
	if s == "" {
		return "Component"
	}
	if r, _ := utf8.DecodeRuneInString(s); !unicode.IsLetter(r) {
		s = "C" + s
	}
	return s
}

// Props returns the props type name for a component: Card → CardProps.
func Props(component string) string { return component + "Props" }

// Field converts an attribute name to a props field name: max-width → MaxWidth.
func Field(attr string) string {
	var b strings.Builder
	for _, seg := range strings.Split(attr, "-") {
		b.WriteString(upperFirst(seg))
	}
	return b.String()
}

func upperFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}
