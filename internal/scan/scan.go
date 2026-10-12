// Package scan implements a Go code lexer with automatic ';' insertion and
// tag-start detection based on the prefix-position rule.
//
// The lexer does not parse markup: on '<' in prefix position it returns
// TagStart without consuming the character. The parser parses the tag itself
// and moves the lexer past it via Reset.
package scan

import (
	"unicode"
	"unicode/utf8"

	"github.com/redexp/gostro/internal/token"
)

// Scanner yields significant tokens (whitespace and comments are skipped).
type Scanner struct {
	src    []byte
	off    int
	prefix bool // a following '<' without '-' starts a tag
	semi   bool // a newline inserts ';'
}

// New creates a lexer at the start of src (in prefix position).
func New(src []byte) *Scanner {
	return &Scanner{src: src, prefix: true}
}

// Offset returns the current offset.
func (s *Scanner) Offset() int { return s.off }

// Reset moves the lexer to off with the given state.
// prefix reports whether the position is a prefix position; semi whether a newline inserts ';'.
func (s *Scanner) Reset(off int, prefix, semi bool) {
	s.off = off
	s.prefix = prefix
	s.semi = semi
}

// ResetStatement resets the lexer to the start of a new statement.
func (s *Scanner) ResetStatement(off int) { s.Reset(off, true, false) }

// ResetAfterOperand resets the lexer to the position right after an operand (e.g. a tag expression).
func (s *Scanner) ResetAfterOperand(off int) { s.Reset(off, false, true) }

// Next returns the next significant token.
func (s *Scanner) Next() token.Token {
	t := s.next()
	s.prefix = token.PrefixAfter(t)
	s.semi = token.SemiAfter(t)
	return t
}

func (s *Scanner) autoSemi(off, end int) token.Token {
	return token.Token{Kind: token.Semi, Off: off, End: end, Auto: true}
}

func (s *Scanner) next() token.Token {
	// Skip whitespace and comments, honoring ASI.
	for s.off < len(s.src) {
		c := s.src[s.off]
		switch {
		case c == '\n':
			if s.semi {
				s.off++
				return s.autoSemi(s.off-1, s.off)
			}
			s.off++
		case c == ' ' || c == '\t' || c == '\r':
			s.off++
		case c == '/' && s.off+1 < len(s.src) && s.src[s.off+1] == '/':
			// Line comment up to the end of the line; '\n' is handled by the loop.
			for s.off < len(s.src) && s.src[s.off] != '\n' {
				s.off++
			}
		case c == '/' && s.off+1 < len(s.src) && s.src[s.off+1] == '*':
			start := s.off
			end := indexFrom(s.src, s.off+2, "*/")
			if end < 0 {
				s.off = len(s.src)
				return token.Token{Kind: token.Illegal, Off: start, End: s.off, Text: string(s.src[start:]), Err: "comment is not closed"}
			}
			s.off = end + 2
			if s.semi && containsNewline(s.src[start:s.off]) {
				// A multi-line comment acts as a newline.
				return s.autoSemi(start, s.off)
			}
		default:
			goto token
		}
	}
	if s.semi {
		return s.autoSemi(s.off, s.off)
	}
	return token.Token{Kind: token.EOF, Off: s.off, End: s.off}

token:
	start := s.off
	c := s.src[s.off]

	if c == '<' && s.prefix && !(s.off+1 < len(s.src) && s.src[s.off+1] == '-') {
		return token.Token{Kind: token.TagStart, Off: start, End: start + 1, Text: "<"}
	}

	r, size := utf8.DecodeRune(s.src[s.off:])
	switch {
	case isLetter(r):
		s.off += size
		for s.off < len(s.src) {
			r, size := utf8.DecodeRune(s.src[s.off:])
			if !isLetter(r) && !isDigit(r) {
				break
			}
			s.off += size
		}
		text := string(s.src[start:s.off])
		if token.IsKeyword(text) {
			return token.Token{Kind: token.Keyword, Off: start, End: s.off, Text: text}
		}
		return token.Token{Kind: token.Ident, Off: start, End: s.off, Text: text}
	case isDecimal(c) || (c == '.' && s.off+1 < len(s.src) && isDecimal(s.src[s.off+1])):
		return s.number()
	case c == '"':
		return s.quoted('"', token.String)
	case c == '\'':
		return s.quoted('\'', token.Char)
	case c == '`':
		end := indexFrom(s.src, s.off+1, "`")
		if end < 0 {
			s.off = len(s.src)
			return token.Token{Kind: token.Illegal, Off: start, End: s.off, Text: string(s.src[start:]), Err: "raw string is not closed"}
		}
		s.off = end + 1
		return token.Token{Kind: token.RawString, Off: start, End: s.off, Text: string(s.src[start:s.off])}
	case c == ';':
		s.off++
		return token.Token{Kind: token.Semi, Off: start, End: s.off, Text: ";"}
	}

	for _, op := range token.Operators {
		if hasPrefixAt(s.src, s.off, op) {
			s.off += len(op)
			return token.Token{Kind: token.Op, Off: start, End: s.off, Text: op}
		}
	}

	s.off += size
	return token.Token{Kind: token.Illegal, Off: start, End: s.off, Text: string(s.src[start:s.off]), Err: "unexpected character " + quoteRune(r)}
}

func (s *Scanner) number() token.Token {
	start := s.off
	kind := token.Int
	if s.src[s.off] == '0' && s.off+1 < len(s.src) && (s.src[s.off+1]|0x20 == 'x' || s.src[s.off+1]|0x20 == 'b' || s.src[s.off+1]|0x20 == 'o') {
		hex := s.src[s.off+1]|0x20 == 'x'
		s.off += 2
		for s.off < len(s.src) {
			c := s.src[s.off]
			if isHex(c) || c == '_' {
				s.off++
			} else if hex && c == '.' {
				kind = token.Float
				s.off++
			} else if hex && c|0x20 == 'p' {
				kind = token.Float
				s.off++
				if s.off < len(s.src) && (s.src[s.off] == '+' || s.src[s.off] == '-') {
					s.off++
				}
			} else {
				break
			}
		}
	} else {
		for s.off < len(s.src) {
			c := s.src[s.off]
			if isDecimal(c) || c == '_' {
				s.off++
			} else if c == '.' {
				kind = token.Float
				s.off++
			} else if c|0x20 == 'e' {
				kind = token.Float
				s.off++
				if s.off < len(s.src) && (s.src[s.off] == '+' || s.src[s.off] == '-') {
					s.off++
				}
			} else {
				break
			}
		}
	}
	if s.off < len(s.src) && s.src[s.off] == 'i' {
		kind = token.Imag
		s.off++
	}
	return token.Token{Kind: kind, Off: start, End: s.off, Text: string(s.src[start:s.off])}
}

func (s *Scanner) quoted(q byte, kind token.Kind) token.Token {
	start := s.off
	s.off++
	for s.off < len(s.src) {
		c := s.src[s.off]
		switch c {
		case '\\':
			s.off += 2
			continue
		case '\n':
			return token.Token{Kind: token.Illegal, Off: start, End: s.off, Text: string(s.src[start:s.off]), Err: "literal is not closed"}
		case q:
			s.off++
			return token.Token{Kind: kind, Off: start, End: s.off, Text: string(s.src[start:s.off])}
		}
		s.off++
	}
	s.off = len(s.src)
	return token.Token{Kind: token.Illegal, Off: start, End: s.off, Text: string(s.src[start:]), Err: "literal is not closed"}
}

// SkipSpace returns the offset of the first character after whitespace and
// comments, starting at off. Newlines are skipped as well.
func SkipSpace(src []byte, off int) int {
	for off < len(src) {
		c := src[off]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			off++
		case c == '/' && off+1 < len(src) && src[off+1] == '/':
			for off < len(src) && src[off] != '\n' {
				off++
			}
		case c == '/' && off+1 < len(src) && src[off+1] == '*':
			end := indexFrom(src, off+2, "*/")
			if end < 0 {
				return len(src)
			}
			off = end + 2
		default:
			return off
		}
	}
	return off
}

// IsIdentStart reports whether r can start a Go identifier.
func IsIdentStart(r rune) bool { return isLetter(r) }

// IsIdentPart reports whether r can be part of a Go identifier.
func IsIdentPart(r rune) bool { return isLetter(r) || isDigit(r) }

func isLetter(r rune) bool {
	return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || r == '_' || r >= utf8.RuneSelf && unicode.IsLetter(r)
}

func isDigit(r rune) bool {
	return '0' <= r && r <= '9' || r >= utf8.RuneSelf && unicode.IsDigit(r)
}

func isDecimal(c byte) bool { return '0' <= c && c <= '9' }

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c|0x20 && c|0x20 <= 'f'
}

func hasPrefixAt(src []byte, off int, p string) bool {
	if len(src)-off < len(p) {
		return false
	}
	for i := 0; i < len(p); i++ {
		if src[off+i] != p[i] {
			return false
		}
	}
	return true
}

func indexFrom(src []byte, off int, p string) int {
	for i := off; i+len(p) <= len(src); i++ {
		if hasPrefixAt(src, i, p) {
			return i
		}
	}
	return -1
}

func containsNewline(b []byte) bool {
	for _, c := range b {
		if c == '\n' {
			return true
		}
	}
	return false
}

func quoteRune(r rune) string {
	if r == utf8.RuneError {
		return "(invalid UTF-8)"
	}
	return "'" + string(r) + "'"
}
