package scan

import (
	"strings"
	"testing"

	"github.com/redexp/gostro/internal/token"
)

func tokens(src string) []token.Token {
	s := New([]byte(src))
	var out []token.Token
	for i := 0; i < 1000; i++ {
		t := s.Next()
		if t.Kind == token.EOF {
			break
		}
		if t.Kind == token.TagStart {
			// Mimic the parser: skip the tag up to '>' and continue as after an operand.
			end := strings.IndexByte(src[t.Off:], '>')
			if end < 0 {
				out = append(out, t)
				break
			}
			out = append(out, t)
			s.ResetAfterOperand(t.Off + end + 1)
			continue
		}
		out = append(out, t)
	}
	return out
}

func describe(ts []token.Token) string {
	var b strings.Builder
	for i, t := range ts {
		if i > 0 {
			b.WriteByte(' ')
		}
		switch t.Kind {
		case token.Semi:
			if t.Auto {
				b.WriteString("⏎")
			} else {
				b.WriteString(";")
			}
		case token.TagStart:
			b.WriteString("TAG")
		case token.Op, token.Ident, token.Keyword:
			b.WriteString(t.Text)
		default:
			b.WriteString(t.Kind.String())
		}
	}
	return b.String()
}

// Table 3.4 of the spec.
func TestPrefixRule(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"if x < y {", "if x < y {"},
		{"a := b << 2", "a := b << Int ⏎"},
		{"a <<= 2", "a <<= Int ⏎"},
		{"v := <-ch", "v := <- ch ⏎"},
		{"func f() <-chan int", "func f ( ) <- chan int ⏎"},
		{"for i := 0; i < n; i++ {", "for i := Int ; i < n ; i ++ {"},
		{"x := <div/>", "x := TAG ⏎"},
		{"return <p>{t}</p>", "return TAG { t } TAG ⏎"},
		{"xs := []Node{<li/>, <li/>}", "xs := [ ] Node { TAG , TAG } ⏎"},
		{`m := map[string]int{"a<b": 1}`, "m := map [ string ] int { String : Int } ⏎"},
		{"// <div>", ""},
		{"/* <div> */", ""},
		{`s := "<div>"`, "s := String ⏎"},
		{"s := `<div>`", "s := RawString ⏎"},
	}
	for _, tt := range tests {
		got := describe(tokens(tt.src))
		if got != tt.want {
			t.Errorf("%q:\n got  %s\n want %s", tt.src, got, tt.want)
		}
	}
}

func TestTagCounts(t *testing.T) {
	tests := []struct {
		src  string
		tags int
	}{
		{"x := <div/>", 1},
		// In this simplified test '</p>' is also a TAG: the position after '}' is a prefix position.
		{"return <p>{t}</p>", 2},
		{"xs := []Node{<li/>, <li/>}", 2},
		{`m := map[string]int{"a<b": 1}`, 0},
		{"// <div>", 0},
		{"/* <div> */", 0},
		{`s := "<div>"`, 0},
		{"s := `<div>`", 0},
		{"if x < y {", 0},
		{"v := <-ch", 0},
		{"a := b << 2", 0},
		{"x := a\n<div/>", 1},
		{"case <div/>:", 1},
		{"f(a, <b/>)", 1},
		{"x := !<b/>", 1},
		{"x := y <b", 0},
	}
	for _, tt := range tests {
		n := 0
		for _, tok := range tokens(tt.src) {
			if tok.Kind == token.TagStart {
				n++
			}
		}
		if n != tt.tags {
			t.Errorf("%q: %d tags, want %d", tt.src, n, tt.tags)
		}
	}
}

func TestASI(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"x\ny", "x ⏎ y ⏎"},
		{"x +\ny", "x + y ⏎"},
		{"return\n", "return ⏎"},
		{"f()\n", "f ( ) ⏎"},
		{"a[i]\n", "a [ i ] ⏎"},
		{"}\n", "} ⏎"},
		{"i++\n", "i ++ ⏎"},
		{"x // c\ny", "x ⏎ y ⏎"},
		{"x /* a\nb */ y", "x ⏎ y ⏎"},
		{"x /* a */ y", "x y ⏎"},
		{"break\ncontinue\nfallthrough\n", "break ⏎ continue ⏎ fallthrough ⏎"},
		{"if x {\n}", "if x { } ⏎"},
	}
	for _, tt := range tests {
		got := describe(tokens(tt.src))
		if got != tt.want {
			t.Errorf("%q:\n got  %s\n want %s", tt.src, got, tt.want)
		}
	}
}

func TestLiterals(t *testing.T) {
	tests := []struct {
		src  string
		kind token.Kind
	}{
		{"123", token.Int},
		{"0x1F", token.Int},
		{"0b1010", token.Int},
		{"0o17", token.Int},
		{"1_000", token.Int},
		{"1.5", token.Float},
		{".5", token.Float},
		{"1e10", token.Float},
		{"1.5e-3", token.Float},
		{"0x1p-2", token.Float},
		{"2i", token.Imag},
		{`'a'`, token.Char},
		{`'\''`, token.Char},
		{`"a\"b"`, token.String},
		{"`a\nb`", token.RawString},
		{"naïve", token.Ident},
		{"日本", token.Ident},
	}
	for _, tt := range tests {
		s := New([]byte(tt.src))
		tok := s.Next()
		if tok.Kind != tt.kind || tok.Text != tt.src {
			t.Errorf("%q: %v %q", tt.src, tok.Kind, tok.Text)
		}
	}
}

func TestIllegal(t *testing.T) {
	for _, src := range []string{`"abc`, "`abc", "/* abc", "'a", "#", "\"a\nb\""} {
		s := New([]byte(src))
		found := false
		for i := 0; i < 100; i++ {
			tok := s.Next()
			if tok.Kind == token.Illegal {
				found = true
				break
			}
			if tok.Kind == token.EOF {
				break
			}
		}
		if !found {
			t.Errorf("%q: expected Illegal", src)
		}
	}
}

func FuzzScanner(f *testing.F) {
	for _, s := range []string{
		"x := <div/>", "if x < y {", "v := <-ch", "`raw", `"str`, "/* c", "0x1p-2i",
		"return <p>{t}</p>", "'\\", "\xff\xfe", "naïve := 1",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		s := New([]byte(src))
		for i := 0; i < len(src)+10; i++ {
			tok := s.Next()
			if tok.Off < 0 || tok.End > len(src) || tok.Off > tok.End {
				t.Fatalf("invalid token bounds %+v for %q", tok, src)
			}
			if tok.Kind == token.EOF {
				return
			}
			if tok.Kind == token.TagStart {
				s.ResetAfterOperand(tok.End)
			}
		}
		t.Fatalf("lexer did not reach EOF on %q", src)
	})
}
