package parse

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/redexp/gostro/internal/token"
)

// Error is a parse error in Go format: `file:line:col: message`.
type Error struct {
	File string
	Pos  token.Pos
	Msg  string
	Hint string // optional hint with a suggested fix

	line      string // source line for the underline
	underline int    // underline length in bytes (0 means no underline)
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Pos.Line, e.Pos.Col, e.Msg)
}

// Pretty returns the message together with the source line, underline, and hint.
func (e *Error) Pretty() string {
	var b strings.Builder
	b.WriteString(e.Error())
	if e.underline > 0 && e.line != "" {
		num := fmt.Sprint(e.Pos.Line)
		pad := strings.Repeat(" ", len(num))
		line := strings.ReplaceAll(e.line, "\t", "    ")
		fmt.Fprintf(&b, "\n %s | %s\n %s | ", num, line, pad)
		prefix := e.line[:min(e.Pos.Col-1, len(e.line))]
		width := utf8.RuneCountInString(strings.ReplaceAll(prefix, "\t", "    "))
		end := min(e.Pos.Col-1+e.underline, len(e.line))
		marks := max(1, utf8.RuneCountInString(e.line[min(e.Pos.Col-1, len(e.line)):end]))
		b.WriteString(strings.Repeat(" ", width))
		b.WriteString(strings.Repeat("^", marks))
	}
	if e.Hint != "" {
		b.WriteString("\n hint: ")
		b.WriteString(e.Hint)
	}
	return b.String()
}
