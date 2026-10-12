// Package token defines the tokens of the Go subset understood by gostro,
// the automatic semicolon insertion (ASI) rules, and the prefix-position
// rule that distinguishes the start of a tag from the `<` operator.
package token

import "strconv"

// Kind is a token kind.
type Kind int

const (
	EOF Kind = iota
	Illegal
	Ident
	Keyword
	Int
	Float
	Imag
	Char
	String    // "…"
	RawString // `…`
	Op        // operators and punctuation, except ';'
	Semi      // ';', explicit or automatically inserted (Auto)
	TagStart  // '<' in prefix position, not followed by '-'
)

var kindNames = [...]string{
	EOF:       "EOF",
	Illegal:   "Illegal",
	Ident:     "Ident",
	Keyword:   "Keyword",
	Int:       "Int",
	Float:     "Float",
	Imag:      "Imag",
	Char:      "Char",
	String:    "String",
	RawString: "RawString",
	Op:        "Op",
	Semi:      "Semi",
	TagStart:  "TagStart",
}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "Kind(" + strconv.Itoa(int(k)) + ")"
}

// IsLiteral reports whether the kind is a basic literal.
func (k Kind) IsLiteral() bool {
	return k >= Int && k <= RawString
}

// Token is a single significant token. Text is a slice of the source text.
type Token struct {
	Kind Kind
	Text string
	Off  int  // offset of the first byte
	End  int  // offset past the last byte
	Auto bool // for Semi: inserted automatically
	Err  string
}

// Is reports whether the token has the given kind and text.
func (t Token) Is(k Kind, text string) bool { return t.Kind == k && t.Text == text }

// IsOp reports whether the token is the operator text.
func (t Token) IsOp(text string) bool { return t.Kind == Op && t.Text == text }

// IsKeyword reports whether the token is the keyword text.
func (t Token) IsKeyword(text string) bool { return t.Kind == Keyword && t.Text == text }

var keywords = map[string]bool{
	"break": true, "case": true, "chan": true, "const": true, "continue": true,
	"default": true, "defer": true, "else": true, "fallthrough": true, "for": true,
	"func": true, "go": true, "goto": true, "if": true, "import": true,
	"interface": true, "map": true, "package": true, "range": true, "return": true,
	"select": true, "struct": true, "switch": true, "type": true, "var": true,
}

// IsKeyword reports whether s is a Go keyword.
func IsKeyword(s string) bool { return keywords[s] }

// Operators lists all Go operators and punctuation (except ';'),
// ordered so that longer ones come first when they share a prefix.
var Operators = []string{
	"<<=", ">>=", "&^=", "...",
	"&&", "||", "<-", "++", "--", "==", "!=", "<=", ">=", ":=",
	"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<", ">>", "&^",
	"+", "-", "*", "/", "%", "&", "|", "^", "<", ">", "=", "!", "~",
	"(", ")", "[", "]", "{", "}", ",", ".", ":",
}

// PrefixAfter reports whether the position after token t is a prefix position,
// i.e. a following '<' (not followed by '-') starts a tag (spec, section 3.2).
func PrefixAfter(t Token) bool {
	switch t.Kind {
	case EOF, Semi:
		return true
	case Ident, Int, Float, Imag, Char, String, RawString, TagStart:
		// TagStart here means "a tag expression just ended", which is an operand.
		return false
	case Keyword:
		switch t.Text {
		case "break", "continue", "fallthrough":
			return false
		}
		return true
	case Op:
		switch t.Text {
		case ")", "]", "++", "--":
			return false
		}
		// Per the spec, '}' is a prefix position: a '<' comparison cannot
		// follow the end of a block or composite literal in Go.
		return true
	}
	return true
}

// SemiAfter reports whether a newline after token t inserts a ';'.
func SemiAfter(t Token) bool {
	switch t.Kind {
	case Ident, Int, Float, Imag, Char, String, RawString, TagStart:
		return true
	case Keyword:
		switch t.Text {
		case "break", "continue", "fallthrough", "return":
			return true
		}
	case Op:
		switch t.Text {
		case ")", "]", "}", "++", "--":
			return true
		}
	}
	return false
}

// Pos is a position in the source file; Line and Col are 1-based, Col is in bytes (as in Go).
type Pos struct {
	Off  int
	Line int
	Col  int
}

// LineTable converts offsets to line and column.
type LineTable struct {
	starts []int
}

// NewLineTable builds the table of line start offsets.
func NewLineTable(src []byte) *LineTable {
	starts := []int{0}
	for i, c := range src {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return &LineTable{starts: starts}
}

// Pos returns the position for offset off.
func (lt *LineTable) Pos(off int) Pos {
	lo, hi := 0, len(lt.starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if lt.starts[mid] <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return Pos{Off: off, Line: lo + 1, Col: off - lt.starts[lo] + 1}
}

// LineStart returns the start offset of line (1-based).
func (lt *LineTable) LineStart(line int) int {
	if line < 1 {
		return 0
	}
	if line > len(lt.starts) {
		return lt.starts[len(lt.starts)-1]
	}
	return lt.starts[line-1]
}
