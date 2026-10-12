package runtime

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

var htmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	"'", "&#39;",
	`"`, "&#34;",
)

// EscapeString escapes & < > ' " in text and attribute values.
func EscapeString(s string) string {
	if !strings.ContainsAny(s, `&<>'"`) {
		return s
	}
	return htmlEscaper.Replace(s)
}

// stringify converts a scalar value to a string. ok is false for unsupported types.
func stringify(v any) (s string, ok bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case SafeURL:
		return string(x), true
	case bool:
		return strconv.FormatBool(x), true
	case int:
		return strconv.Itoa(x), true
	case int8:
		return strconv.FormatInt(int64(x), 10), true
	case int16:
		return strconv.FormatInt(int64(x), 10), true
	case int32:
		return strconv.FormatInt(int64(x), 10), true
	case int64:
		return strconv.FormatInt(x, 10), true
	case uint:
		return strconv.FormatUint(uint64(x), 10), true
	case uint8:
		return strconv.FormatUint(uint64(x), 10), true
	case uint16:
		return strconv.FormatUint(uint64(x), 10), true
	case uint32:
		return strconv.FormatUint(uint64(x), 10), true
	case uint64:
		return strconv.FormatUint(x, 10), true
	case uintptr:
		return strconv.FormatUint(uint64(x), 10), true
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32), true
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64), true
	case complex64, complex128:
		return fmt.Sprint(x), true
	case fmt.Stringer:
		return x.String(), true
	case error:
		return x.Error(), true
	}
	return "", false
}

// Text writes a value in text context, escaping & < > ' ".
// RawHTML is written as is; nil writes nothing.
func Text(w Writer, v any) {
	switch x := v.(type) {
	case nil:
		return
	case RawHTML:
		w.WriteString(string(x))
		return
	}
	s, ok := stringify(v)
	if !ok {
		w.SetErr(errUnsupported("text", v))
		return
	}
	w.WriteString(EscapeString(s))
}

// Attr writes an attribute value (without quotes; the compiler emits them).
// RawHTML is written unescaped.
func Attr(w Writer, v any) {
	switch x := v.(type) {
	case nil:
		return
	case RawHTML:
		w.WriteString(string(x))
		return
	}
	s, ok := stringify(v)
	if !ok {
		w.SetErr(errUnsupported("attribute", v))
		return
	}
	w.WriteString(EscapeString(s))
}

// UnsafeURLReplacement is substituted for URLs with a forbidden scheme.
const UnsafeURLReplacement = "about:invalid#gostro"

// URL writes the value of a URL attribute (href, src, …). The javascript:,
// vbscript: and data: schemes (except data:image/*) are replaced with
// about:invalid#gostro. SafeURL is written without checking.
func URL(w Writer, v any) {
	switch x := v.(type) {
	case nil:
		return
	case SafeURL:
		w.WriteString(EscapeString(string(x)))
		return
	}
	s, ok := stringify(v)
	if !ok {
		w.SetErr(errUnsupported("URL", v))
		return
	}
	if !IsSafeURL(s) {
		s = UnsafeURLReplacement
	}
	w.WriteString(EscapeString(s))
}

// IsSafeURL checks the URL scheme. Browsers ignore leading control characters
// and spaces and tabs/newlines inside the scheme, so they are stripped before checking.
func IsSafeURL(s string) bool {
	var b strings.Builder
	for _, r := range s {
		if r <= ' ' || r == 0x7f {
			continue // spaces and control characters
		}
		if r == ':' || r == '/' || r == '?' || r == '#' {
			b.WriteRune(r)
			break
		}
		b.WriteRune(r)
		if b.Len() > 32 {
			break
		}
	}
	head := strings.ToLower(b.String())
	i := strings.IndexByte(head, ':')
	if i < 0 {
		return true // relative URL
	}
	switch scheme := head[:i]; scheme {
	case "javascript", "vbscript":
		return false
	case "data":
		rest := strings.ToLower(strings.TrimLeft(s[strings.IndexByte(s, ':')+1:], " \t\r\n"))
		return strings.HasPrefix(rest, "image/")
	}
	return true
}

// JSON serializes v for embedding inside <script>. encoding/json escapes
// < > & and U+2028/U+2029, so the output cannot close the </script> tag.
func JSON(v any) RawHTML {
	b, err := json.Marshal(v)
	if err != nil {
		return RawHTML("null")
	}
	return RawHTML(b)
}

// ClassList writes a class:list value: elements of []any are joined with spaces.
// A string is used as is; []string element by element; map[string]bool yields
// the keys set to true (sorted); nil and empty strings are skipped.
func ClassList(w Writer, v []any) {
	var parts []string
	var add func(x any)
	add = func(x any) {
		switch x := x.(type) {
		case nil:
		case string:
			if s := strings.TrimSpace(x); s != "" {
				parts = append(parts, s)
			}
		case []string:
			for _, s := range x {
				add(s)
			}
		case []any:
			for _, s := range x {
				add(s)
			}
		case map[string]bool:
			keys := make([]string, 0, len(x))
			for k, on := range x {
				if on {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			for _, k := range keys {
				add(k)
			}
		default:
			if s, ok := stringify(x); ok {
				add(s)
			} else {
				w.SetErr(errUnsupported("class:list", x))
			}
		}
	}
	for _, x := range v {
		add(x)
	}
	w.WriteString(EscapeString(strings.Join(parts, " ")))
}

// BoolAttr writes a boolean attribute (disabled, checked, …) with a dynamic value:
// for bool the attribute is present only when true; nil omits it; anything else
// yields name="value". prefix holds a space and the attribute name, e.g. " disabled".
func BoolAttr(w Writer, prefix string, v any) {
	switch x := v.(type) {
	case nil:
		return
	case bool:
		if x {
			w.WriteString(prefix)
		}
		return
	}
	w.WriteString(prefix)
	w.WriteString(`="`)
	Attr(w, v)
	w.WriteString(`"`)
}
