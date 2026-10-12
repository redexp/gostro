// Package parse parses .gostro files: Go code with embedded tags.
//
// Go code is not fully parsed: the generator only needs a lexer with ASI and
// the prefix-position rule. Statements, control-flow clauses, and expressions
// are passed through as opaque text spans (ast.Code).
package parse

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/redexp/gostro/internal/ast"
	"github.com/redexp/gostro/internal/scan"
	"github.com/redexp/gostro/internal/token"
)

// ParseFile parses a file. name is the base file name used in messages and //line
// directives; key is the prefix for style and script registration keys (usually the path within the module).
func ParseFile(name, key string, src []byte) (f *ast.File, err error) {
	p := &parser{
		file: name,
		key:  key,
		src:  src,
		lt:   token.NewLineTable(src),
		sc:   scan.New(src),
	}
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(*Error); ok {
				f, err = nil, e
				return
			}
			panic(r)
		}
	}()
	return p.parseFile(), nil
}

type parser struct {
	file    string
	key     string
	src     []byte
	lt      *token.LineTable
	sc      *scan.Scanner
	styleN  int
	scriptN int
}

// mode is the parsing mode for a statement list.
type mode struct {
	markup    bool // '{' is interpolation; bare text is checked
	labels    bool // `Name:` labels are allowed
	inElement bool // inside an element: '</' closes it
	inSwitch  bool // stop at case/default
}

var (
	// bodyMode is a component body: a list of children, so '{' is interpolation.
	bodyMode = mode{markup: true, labels: true}
	// goMode is a plain function body without statement tags: '{' is a Go block.
	goMode = mode{labels: true}
	// childMode is for element children.
	childMode = mode{markup: true, inElement: true}
)

// block returns the mode for a control-flow body.
func (m mode) block() mode {
	m.inSwitch = false
	return m
}

// --- errors ------------------------------------------------------------------

func (p *parser) pos(off int) token.Pos { return p.lt.Pos(off) }

func (p *parser) newError(off int, msg string) *Error {
	pos := p.pos(off)
	start := p.lt.LineStart(pos.Line)
	end := start
	for end < len(p.src) && p.src[end] != '\n' {
		end++
	}
	return &Error{File: p.file, Pos: pos, Msg: msg, line: strings.TrimRight(string(p.src[start:end]), "\r")}
}

func (p *parser) fail(off int, format string, args ...any) {
	panic(p.newError(off, fmt.Sprintf(format, args...)))
}

func (p *parser) failUnderline(off, length int, hint, format string, args ...any) {
	e := p.newError(off, fmt.Sprintf(format, args...))
	e.underline = max(1, length)
	e.Hint = hint
	panic(e)
}

func (p *parser) posString(off int) string {
	pos := p.pos(off)
	return fmt.Sprintf("%d:%d", pos.Line, pos.Col)
}

// --- helpers -------------------------------------------------------------

func (p *parser) skipSpace(off int) int { return scan.SkipSpace(p.src, off) }

// skipSpaceSemis skips whitespace, comments, and explicit ';'.
func (p *parser) skipSpaceSemis(off int) int {
	for {
		off = p.skipSpace(off)
		if off < len(p.src) && p.src[off] == ';' {
			off++
			continue
		}
		return off
	}
}

func (p *parser) at(off int, s string) bool {
	return off+len(s) <= len(p.src) && string(p.src[off:off+len(s)]) == s
}

// word returns the identifier starting at off.
func (p *parser) word(off int) string {
	end := off
	for end < len(p.src) {
		r, size := utf8.DecodeRune(p.src[end:])
		if !scan.IsIdentPart(r) {
			break
		}
		end += size
	}
	return string(p.src[off:end])
}

func (p *parser) text(start, end int) string { return string(p.src[start:end]) }

// --- file -----------------------------------------------------------------

func (p *parser) parseFile() *ast.File {
	f := &ast.File{Name: p.file, Key: p.key}
	off := 0
	first := true
	for {
		off = p.skipSpaceSemis(off)
		if off >= len(p.src) {
			break
		}
		if p.src[off] == '}' {
			p.fail(off, "unexpected '}'")
		}
		if p.at(off, "</") {
			p.failCloseWithoutOpen(off)
		}
		w := p.word(off)
		switch {
		case w == "package":
			if !first {
				p.fail(off, "package directive is only allowed on the first line of the file")
			}
			off = p.parsePackage(f, off)
		case w == "import":
			off = p.parseImport(f, off)
		case w == "func" && p.isFuncDecl(off):
			fn, end := p.parseFunc(off)
			f.Funcs = append(f.Funcs, fn)
			off = end
		case w == "type" && p.isPropsDecl(off):
			off = p.parseProps(f, off)
		default:
			if !f.HasBody {
				f.HasBody = true
				f.BodyAt = p.pos(off)
			}
			nodes, end := p.parseItem(off, bodyMode)
			f.Body = append(f.Body, nodes...)
			off = end
		}
		first = false
	}
	return f
}

func (p *parser) parsePackage(f *ast.File, off int) int {
	p.sc.ResetStatement(off)
	p.sc.Next() // package
	name := p.sc.Next()
	if name.Kind != token.Ident {
		p.fail(name.Off, "expected package name after package")
	}
	f.Package = name.Text
	f.PackageAt = p.pos(off)
	return p.expectSemi()
}

func (p *parser) expectSemi() int {
	t := p.sc.Next()
	if t.Kind != token.Semi && t.Kind != token.EOF {
		p.fail(t.Off, "expected end of line, found %q", t.Text)
	}
	return t.End
}

func (p *parser) parseImport(f *ast.File, off int) int {
	p.sc.ResetStatement(off)
	p.sc.Next() // import
	t := p.sc.Next()
	var end int
	if t.IsOp("(") {
		depth := 1
		for depth > 0 {
			t = p.sc.Next()
			switch {
			case t.Kind == token.EOF:
				p.fail(off, "import block is not closed")
			case t.Kind == token.Illegal:
				p.fail(t.Off, "%s", t.Err)
			case t.IsOp("("):
				depth++
			case t.IsOp(")"):
				depth--
			}
		}
		end = t.End
	} else {
		if t.Kind == token.Ident || t.IsOp(".") {
			t = p.sc.Next()
		}
		if t.Kind != token.String && t.Kind != token.RawString {
			p.fail(t.Off, "expected quoted import path")
		}
		end = t.End
	}
	f.Imports = append(f.Imports, ast.Import{At: ast.At{P: p.pos(off)}, Text: p.text(off, end)})
	p.expectSemi()
	return p.sc.Offset()
}

// isFuncDecl distinguishes a func declaration from a statement with a function literal.
func (p *parser) isFuncDecl(off int) bool {
	p.sc.ResetStatement(off)
	p.sc.Next() // func
	t := p.sc.Next()
	if t.Kind == token.Ident {
		return true
	}
	if !t.IsOp("(") {
		return false
	}
	// func (r T) Name(…) is a method; func (…) {…}() is a literal.
	depth := 1
	for depth > 0 {
		t = p.sc.Next()
		switch {
		case t.Kind == token.EOF || t.Kind == token.Illegal:
			return false
		case t.IsOp("("):
			depth++
		case t.IsOp(")"):
			depth--
		}
	}
	return p.sc.Next().Kind == token.Ident
}

// isPropsDecl recognizes `type XxxProps struct`, the component props types
// that are hoisted to package level. `type Props` is not special and, like
// any other type, stays in the component body.
func (p *parser) isPropsDecl(off int) bool {
	p.sc.ResetStatement(off)
	p.sc.Next() // type
	name := p.sc.Next()
	st := p.sc.Next()
	return name.Kind == token.Ident && len(name.Text) > len("Props") && strings.HasSuffix(name.Text, "Props") && st.IsKeyword("struct")
}

func (p *parser) parseProps(f *ast.File, off int) int {
	p.sc.ResetStatement(off)
	p.sc.Next() // type
	name := p.sc.Next()
	st := p.sc.Next()
	open := p.sc.Next()
	if !open.IsOp("{") {
		p.fail(open.Off, "expected '{' after struct")
	}
	end := p.skipBalanced(open)
	props := &ast.Props{At: ast.At{P: p.pos(off)}, Name: name.Text, Type: ast.Code{Parts: []ast.CodePart{{Text: p.text(st.Off, end), At: p.pos(st.Off)}}}}
	f.PropsTypes = append(f.PropsTypes, props)
	p.sc.ResetAfterOperand(end)
	p.expectSemi()
	return p.sc.Offset()
}

// skipBalanced skips tokens up to the bracket matching open; it returns the offset after it.
func (p *parser) skipBalanced(open token.Token) int {
	var stack []string
	stack = append(stack, open.Text)
	for len(stack) > 0 {
		t := p.sc.Next()
		switch t.Kind {
		case token.EOF:
			p.fail(open.Off, "bracket %q is not closed", open.Text)
		case token.Illegal:
			p.fail(t.Off, "%s", t.Err)
		case token.TagStart:
			p.fail(t.Off, "tag is not allowed here")
		case token.Op:
			switch t.Text {
			case "(", "[", "{":
				stack = append(stack, t.Text)
			case ")", "]", "}":
				if closing(stack[len(stack)-1]) != t.Text {
					p.fail(t.Off, "unexpected %q", t.Text)
				}
				stack = stack[:len(stack)-1]
				if len(stack) == 0 {
					return t.End
				}
			}
		}
	}
	return p.sc.Offset()
}

func closing(open string) string {
	switch open {
	case "(":
		return ")"
	case "[":
		return "]"
	}
	return "}"
}

// parseFunc parses a top-level function.
func (p *parser) parseFunc(off int) (*ast.Func, int) {
	fn := &ast.Func{At: ast.At{P: p.pos(off)}}
	p.sc.ResetStatement(off)
	p.sc.Next() // func
	t := p.sc.Next()
	if t.IsOp("(") { // receiver
		p.skipBalanced(t)
		t = p.sc.Next()
	}
	fn.Name = t.Text
	t = p.sc.Next()
	if t.IsOp("[") { // type parameters
		p.skipBalanced(t)
		t = p.sc.Next()
	}
	if !t.IsOp("(") {
		p.fail(t.Off, "expected parameter list for function %s", fn.Name)
	}
	paramsOpen := t.End
	paramsClose := p.skipBalanced(t) - 1
	params := strings.TrimSpace(p.text(paramsOpen, paramsClose))

	// The result extends up to the body '{' (struct{…}/interface{…} braces are part of the type).
	hasResult := false
	var bodyOpen int
	for {
		t = p.sc.Next()
		if t.IsOp("{") {
			bodyOpen = t.Off
			break
		}
		switch {
		case t.Kind == token.EOF || t.Kind == token.Semi:
			p.fail(off, "function %s has no body", fn.Name)
		case t.Kind == token.Illegal:
			p.fail(t.Off, "%s", t.Err)
		case t.IsKeyword("struct") || t.IsKeyword("interface"):
			hasResult = true
			open := p.sc.Next()
			if open.IsOp("{") {
				p.skipBalanced(open)
			}
		case t.IsOp("(") || t.IsOp("["):
			hasResult = true
			p.skipBalanced(t)
		default:
			hasResult = true
		}
	}

	// The body is first parsed as plain Go; if it contains tags in
	// statement position, the function is a component and the body is
	// reparsed as a list of children ('{' is interpolation).
	styleN, scriptN := p.styleN, p.scriptN
	body, end := p.parseList(bodyOpen+1, goMode)
	component := hasStatementTags(body)
	if component {
		p.styleN, p.scriptN = styleN, scriptN
		body, end = p.parseList(bodyOpen+1, bodyMode)
	}
	if end >= len(p.src) || p.src[end] != '}' {
		if p.at(end, "</") {
			p.failCloseWithoutOpen(end)
		}
		p.fail(bodyOpen, "body of function %s is not closed", fn.Name)
	}
	end++

	fn.Body = body
	if component {
		if hasResult {
			p.fail(off, "function %s has tags in statement position, so it is a component and cannot have an explicit result", fn.Name)
		}
		fn.Component = true
		fn.Sig = ast.Code{Parts: []ast.CodePart{{Text: strings.TrimRight(p.text(off, paramsClose), " \t\r\n"), At: p.pos(off)}}}
		fn.AddSlots = !strings.Contains(params, "__gostro.Slots") && isSingleParam(params, fn.Name+"Props")
	} else {
		fn.Sig = ast.Code{Parts: []ast.CodePart{{Text: strings.TrimRight(p.text(off, bodyOpen), " \t\r\n"), At: p.pos(off)}}}
	}
	p.sc.ResetAfterOperand(end)
	t = p.sc.Next()
	if t.Kind != token.Semi && t.Kind != token.EOF {
		p.fail(t.Off, "expected end of line after function %s", fn.Name)
	}
	return fn, t.End
}

// isSingleParam reports whether the parameter list is exactly one parameter of type typ.
func isSingleParam(params, typ string) bool {
	if strings.Contains(params, ",") {
		return false
	}
	fields := strings.Fields(params)
	return len(fields) == 2 && fields[1] == typ
}

// hasStatementTags reports whether the statements contain tags (not just Go code).
func hasStatementTags(nodes []ast.Node) bool {
	for _, n := range nodes {
		switch n := n.(type) {
		case *ast.GoChunk:
		case *ast.Control:
			if hasStatementTags(n.Body) || hasStatementTags(n.Else) {
				return true
			}
		case *ast.Switch:
			for _, c := range n.Cases {
				if hasStatementTags(c.Body) {
					return true
				}
			}
		default:
			return true
		}
	}
	return false
}

// --- statement lists -------------------------------------------------------

// parseList parses statements up to '}', '</', case/default (in a switch), or EOF.
// It returns the offset of the terminator without consuming it.
func (p *parser) parseList(off int, m mode) ([]ast.Node, int) {
	var nodes []ast.Node
	for {
		off = p.skipSpaceSemis(off)
		if off >= len(p.src) || p.src[off] == '}' {
			return nodes, off
		}
		if p.at(off, "</") {
			if !m.inElement {
				p.failCloseWithoutOpen(off)
			}
			return nodes, off
		}
		if m.inSwitch {
			if w := p.word(off); w == "case" || w == "default" {
				return nodes, off
			}
		}
		var items []ast.Node
		items, off = p.parseItem(off, m)
		nodes = append(nodes, items...)
	}
}

// parseItem parses a single statement (spec, section 4.1).
func (p *parser) parseItem(off int, m mode) ([]ast.Node, int) {
	c := p.src[off]
	switch {
	case c == '<' && !p.at(off, "<-"):
		n, end := p.parseTag(off)
		if n == nil {
			return nil, end
		}
		return []ast.Node{n}, end
	case c == '{':
		if m.markup {
			n, end := p.parseInterp(off, ast.CtxText)
			if n == nil {
				return nil, end
			}
			return []ast.Node{n}, end
		}
		return p.parseBlock(off, m)
	}
	switch p.word(off) {
	case "if", "for":
		n, end := p.parseControl(off, m)
		return []ast.Node{n}, end
	case "switch", "select":
		n, end := p.parseSwitch(off, m)
		return []ast.Node{n}, end
	}
	if m.labels {
		if n, end, ok := p.parseLabel(off); ok {
			return []ast.Node{n}, end
		}
	}
	code, end := p.scanGo(off, kStmt, m.markup)
	if len(code.Parts) == 0 {
		return nil, end
	}
	return []ast.Node{&ast.GoChunk{At: ast.At{P: p.pos(off)}, Code: code}}, end
}

// parseLabel recognizes a `Name:` label.
func (p *parser) parseLabel(off int) (ast.Node, int, bool) {
	p.sc.ResetStatement(off)
	id := p.sc.Next()
	if id.Kind != token.Ident {
		return nil, off, false
	}
	colon := p.sc.Next()
	if !colon.IsOp(":") {
		return nil, off, false
	}
	return &ast.GoChunk{At: ast.At{P: p.pos(off)}, Code: ast.Code{Parts: []ast.CodePart{{Text: p.text(off, colon.End), At: p.pos(off)}}}}, colon.End, true
}

func (p *parser) parseBlock(off int, m mode) ([]ast.Node, int) {
	body, end := p.parseList(off+1, m.block())
	if end >= len(p.src) || p.src[end] != '}' {
		p.fail(off, "block '{' is not closed")
	}
	return []ast.Node{&ast.Control{At: ast.At{P: p.pos(off)}, Body: body}}, end + 1
}

// parseControl parses if/for (spec, section 4.2).
func (p *parser) parseControl(off int, m mode) (ast.Node, int) {
	kw := p.word(off)
	clause, open := p.scanGo(off, kClause, m.markup)
	ctl := &ast.Control{At: ast.At{P: p.pos(off)}, Keyword: kw, Clause: clause}
	body, end := p.parseList(open+1, m.block())
	if end >= len(p.src) || p.src[end] != '}' {
		p.failUnclosedBlock(kw, off, end)
	}
	ctl.Body = body
	end++
	if kw != "if" {
		return ctl, end
	}
	// else?
	p.sc.ResetAfterOperand(end)
	t := p.sc.Next()
	if !t.IsKeyword("else") {
		return ctl, end
	}
	ctl.HasElse = true
	ctl.ElseAt = p.pos(t.Off)
	next := p.skipSpace(t.End)
	switch {
	case p.word(next) == "if":
		n, e := p.parseControl(next, m)
		ctl.ElseIf = true
		ctl.Else = []ast.Node{n}
		return ctl, e
	case next < len(p.src) && p.src[next] == '{':
		els, e := p.parseList(next+1, m.block())
		if e >= len(p.src) || p.src[e] != '}' {
			p.failUnclosedBlock("else", t.Off, e)
		}
		ctl.Else = els
		return ctl, e + 1
	}
	p.fail(next, "expected if or '{' after else")
	return nil, 0
}

func (p *parser) failUnclosedBlock(kw string, open, at int) {
	if at < len(p.src) && p.at(at, "</") {
		p.fail(at, "%s block opened at %s is not closed before %s", kw, p.posString(open), p.closeTagText(at))
	}
	p.fail(open, "%s block is not closed: expected '}'", kw)
}

func (p *parser) closeTagText(off int) string {
	end := off
	for end < len(p.src) && p.src[end] != '>' && p.src[end] != '\n' {
		end++
	}
	if end < len(p.src) && p.src[end] == '>' {
		end++
	}
	return p.text(off, end)
}

// parseSwitch parses switch/select with case clauses.
func (p *parser) parseSwitch(off int, m mode) (ast.Node, int) {
	kw := p.word(off)
	clause, open := p.scanGo(off, kClause, m.markup)
	sw := &ast.Switch{At: ast.At{P: p.pos(off)}, Keyword: kw, Clause: clause}
	cur := open + 1
	for {
		cur = p.skipSpaceSemis(cur)
		if cur >= len(p.src) {
			p.failUnclosedBlock(kw, off, cur)
		}
		if p.src[cur] == '}' {
			return sw, cur + 1
		}
		w := p.word(cur)
		if w != "case" && w != "default" {
			if p.at(cur, "</") {
				p.failUnclosedBlock(kw, off, cur)
			}
			p.fail(cur, "expected case or default in %s body", kw)
		}
		cc, end := p.scanGo(cur, kCase, m.markup)
		body, bend := p.parseList(end, mode{markup: m.markup, labels: m.labels, inElement: m.inElement, inSwitch: true})
		sw.Cases = append(sw.Cases, ast.SwitchCase{At: ast.At{P: p.pos(cur)}, Clause: cc, Body: body})
		cur = bend
		if cur < len(p.src) && p.at(cur, "</") {
			p.failUnclosedBlock(kw, off, cur)
		}
	}
}

// --- Go code -----------------------------------------------------------------

type scanKind int

const (
	kStmt   scanKind = iota // up to ';' (consumed), '}' / '</' / EOF (not consumed)
	kClause                 // up to the '{' that opens a block (not consumed)
	kCase                   // up to ':' (consumed, included in the code)
	kInterp                 // up to '}' (not consumed)
)

// scanGo scans Go code from start, collecting text and tag expressions.
// It returns the code and the offset to continue from.
func (p *parser) scanGo(start int, kind scanKind, markup bool) (ast.Code, int) {
	p.sc.ResetStatement(start)
	var (
		code      ast.Code
		textStart = start
		lastEnd   = start
		started   bool
		stack     []string
		toks      []token.Token
		// Bare-text heuristics (only for statements in markup).
		checkBare = markup && kind == kStmt
		declStmt  bool
		prevOp    bool // previous token at depth 0 is an operand
		nTop      int  // number of tokens at depth 0
	)
	flush := func(end int) {
		if end > textStart {
			code.Parts = append(code.Parts, ast.CodePart{Text: p.text(textStart, end), At: p.pos(textStart)})
		}
	}
	bare := func(end int) {
		p.failBareText(start, end)
	}
	for {
		t := p.sc.Next()
		depth := len(stack)
		if !started && t.Kind != token.EOF {
			started = true
			textStart = t.Off // without leading whitespace and comments
		}

		// '</' is never valid Go: it is a closing tag.
		if (t.Kind == token.TagStart || t.IsOp("<")) && p.at(t.Off, "</") && depth == 0 {
			if kind == kStmt {
				if checkBare && nTop > 0 {
					bare(t.Off)
				}
				flush(lastEnd)
				return code, t.Off
			}
			p.failUnexpectedEnd(kind, start, t.Off)
		}

		switch t.Kind {
		case token.EOF:
			if kind == kStmt {
				if checkBare && nTop > 0 {
					bare(t.Off)
				}
				flush(lastEnd)
				return code, t.Off
			}
			p.failUnexpectedEnd(kind, start, t.Off)

		case token.Illegal:
			if checkBare {
				bare(t.Off)
			}
			p.fail(t.Off, "%s", t.Err)

		case token.TagStart:
			flush(t.Off)
			n, end := p.parseTag(t.Off)
			if n == nil {
				p.fail(t.Off, "expected a tag")
			}
			code.Parts = append(code.Parts, ast.CodePart{Tag: n, At: p.pos(t.Off)})
			textStart, lastEnd = end, end
			p.sc.ResetAfterOperand(end)
			tt := token.Token{Kind: token.TagStart, Off: t.Off, End: end}
			toks = append(toks, tt)
			if depth == 0 {
				if checkBare && prevOp && !declStmt {
					bare(t.Off)
				}
				prevOp = true
				nTop++
			}
			continue

		case token.Semi:
			if depth == 0 && kind == kStmt {
				if checkBare && !declStmt && nTop == 1 && prevOp {
					bare(t.Off)
				}
				flush(lastEnd)
				return code, t.End
			}
			if depth == 0 {
				prevOp = false
			}
			continue

		case token.Op:
			switch t.Text {
			case "{":
				if depth == 0 && kind == kClause && !isCompositeBrace(toks, len(toks)) {
					flush(lastEnd)
					return code, t.Off
				}
				stack = append(stack, t.Text)
			case "(", "[":
				stack = append(stack, t.Text)
			case ")", "]", "}":
				if depth == 0 {
					switch {
					case kind == kInterp && t.Text == "}":
						flush(lastEnd)
						return code, t.Off
					case kind == kStmt && t.Text == "}":
						if checkBare && !declStmt && nTop == 1 && prevOp {
							bare(t.Off)
						}
						flush(lastEnd)
						return code, t.Off
					}
					p.fail(t.Off, "unexpected %q", t.Text)
				}
				if closing(stack[depth-1]) != t.Text {
					p.fail(t.Off, "unexpected %q", t.Text)
				}
				stack = stack[:depth-1]
			case ":":
				if depth == 0 && kind == kCase {
					flush(t.End)
					return code, t.End
				}
				if depth == 0 && checkBare && !declStmt {
					bare(t.Off)
				}
			}
		}

		if depth == 0 || len(stack) == 0 {
			operand := t.Kind == token.Ident || t.Kind.IsLiteral()
			if nTop == 0 && t.Kind == token.Keyword {
				declStmt = true
			}
			if checkBare && operand && prevOp && !declStmt {
				bare(t.Off)
			}
			prevOp = operand
			nTop++
		}
		toks = append(toks, t)
		lastEnd = t.End
	}
}

func (p *parser) failUnexpectedEnd(kind scanKind, start, at int) {
	switch kind {
	case kClause:
		p.fail(start, "%s clause has no body: expected '{'", p.word(start))
	case kCase:
		p.fail(start, "%s clause has no ':'", p.word(start))
	case kInterp:
		p.fail(start-1, "interpolation '{' is not closed")
	}
	p.fail(at, "unexpected end of input")
}

// failBareText reports bare text between tags.
func (p *parser) failBareText(start, _ int) {
	// The text extends to the next tag, interpolation, or end of line.
	end := start
	for end < len(p.src) && p.src[end] != '<' && p.src[end] != '{' && p.src[end] != '\n' {
		end++
	}
	txt := strings.TrimSpace(p.text(start, end))
	hint := "text must be inside {\"…\"}"
	if txt != "" {
		hint = "{" + strconv.Quote(txt) + "}"
	}
	p.failUnderline(start, len(txt), hint, "bare text is not allowed; wrap it in {\"…\"}")
}

// isCompositeBrace decides whether the '{' following toks[:i] opens a composite
// literal in an if/for/switch clause. Like the Go parser, only forms with an
// explicit type literal count: []T{…}, [N]T{…}, map[K]V{…}, struct{…}{…}.
func isCompositeBrace(toks []token.Token, i int) bool {
	j := i - 1
	if j < 0 {
		return false
	}
	if toks[j].IsKeyword("struct") || toks[j].IsKeyword("interface") {
		return true // body of a struct{…} / interface{…} type
	}
	if toks[j].IsOp("}") {
		k := matchBack(toks, j)
		return k > 0 && (toks[k-1].IsKeyword("struct") || toks[k-1].IsKeyword("interface"))
	}
	if toks[j].Kind != token.Ident {
		return false
	}
	for j >= 0 && (toks[j].Kind == token.Ident || toks[j].IsOp(".") || toks[j].IsOp("*")) {
		j--
	}
	return j >= 0 && toks[j].IsOp("]") && isTypeBracket(toks, j)
}

// isTypeBracket reports whether the ']' at j closes a type '[' ([]T, [N]T, map[K]V).
func isTypeBracket(toks []token.Token, j int) bool {
	k := matchBack(toks, j)
	if k < 0 {
		return false
	}
	if k == 0 {
		return true
	}
	before := toks[k-1]
	switch {
	case before.IsKeyword("map"):
		return true
	case before.IsOp("]"):
		return isTypeBracket(toks, k-1)
	}
	return token.PrefixAfter(before)
}

// matchBack finds the index of the opening bracket matching toks[j].
func matchBack(toks []token.Token, j int) int {
	want := map[string]string{")": "(", "]": "[", "}": "{"}[toks[j].Text]
	closeText := toks[j].Text
	depth := 0
	for k := j; k >= 0; k-- {
		switch {
		case toks[k].IsOp(closeText):
			depth++
		case toks[k].IsOp(want):
			depth--
			if depth == 0 {
				return k
			}
		}
	}
	return -1
}

// --- markup ------------------------------------------------------------------

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "param": true,
	"source": true, "track": true, "wbr": true,
}

func isNameChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == ':'
}

func (p *parser) failCloseWithoutOpen(off int) {
	p.fail(off, "closing tag %s without an opening tag", p.closeTagText(off))
}

// parseTag parses the tag starting with '<' at off. It returns nil for comments.
func (p *parser) parseTag(off int) (ast.Node, int) {
	start := off
	if p.at(off, "</") {
		p.failCloseWithoutOpen(off)
	}
	if p.at(off, "<!") {
		return p.parseBang(off)
	}
	off++
	nameStart := off
	for off < len(p.src) && isNameChar(p.src[off]) {
		off++
	}
	name := p.text(nameStart, off)
	if name == "" || !isASCIILetter(name[0]) {
		p.fail(start, "expected tag name after '<'")
	}
	attrs, selfClose, off := p.parseAttrs(off, name, start)
	at := ast.At{P: p.pos(start)}

	if isComponentName(name) {
		return p.parseComponent(at, name, attrs, selfClose, off)
	}
	lname := strings.ToLower(name)
	switch lname {
	case "slot":
		return p.parseSlotDef(at, name, attrs, selfClose, off)
	case "style", "script":
		return p.parseRawText(at, lname, attrs, selfClose, off)
	}

	el := &ast.Element{At: at, Name: name, SelfClose: selfClose, Void: voidElements[lname]}
	for i := 0; i < len(attrs); i++ {
		a := attrs[i]
		if a.Name == "set:html" {
			if a.Kind != ast.AttrExpr {
				p.fail(a.P.Off, "set:html expects an expression: set:html={…}")
			}
			code := a.Expr
			el.SetHTML = &code
			continue
		}
		el.Attrs = append(el.Attrs, a)
	}
	if el.Void && el.SetHTML != nil {
		p.fail(start, "set:html is not allowed on void element <%s>", name)
	}
	if selfClose || el.Void {
		return el, off
	}
	children, end := p.parseList(off, childMode)
	if el.SetHTML != nil && len(children) > 0 {
		p.fail(children[0].Pos().Off, "element <%s> with set:html cannot have children", name)
	}
	el.Children = children
	return el, p.expectCloseTag(name, start, end)
}

func isASCIILetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func isComponentName(name string) bool {
	return strings.Contains(name, ".") || name[0] >= 'A' && name[0] <= 'Z'
}

// expectCloseTag checks the closing tag at off and returns the offset after it.
func (p *parser) expectCloseTag(name string, openOff, off int) int {
	if !p.at(off, "</") {
		p.failUnderline(openOff, len(name)+1, "", "tag <%s> is not closed (opened at %s)", name, p.posString(openOff))
	}
	cur := off + 2
	nameStart := cur
	for cur < len(p.src) && isNameChar(p.src[cur]) {
		cur++
	}
	closeName := p.text(nameStart, cur)
	cur = p.skipSpace(cur)
	if cur >= len(p.src) || p.src[cur] != '>' {
		p.fail(off, "closing tag </%s> has no '>'", closeName)
	}
	if closeName != name {
		p.fail(off, "tag </%s> closes <%s> opened at %s", closeName, name, p.posString(openOff))
	}
	return cur + 1
}

func (p *parser) parseBang(off int) (ast.Node, int) {
	if p.at(off, "<!--") {
		end := strings.Index(string(p.src[off+4:]), "-->")
		if end < 0 {
			p.fail(off, "HTML comment is not closed")
		}
		end += off + 4 + 3
		return &ast.Raw{At: ast.At{P: p.pos(off)}, Text: p.text(off, end)}, end
	}
	end := strings.IndexByte(string(p.src[off:]), '>')
	if end < 0 {
		p.fail(off, "tag <! is not closed")
	}
	end += off + 1
	return &ast.Raw{At: ast.At{P: p.pos(off)}, Text: p.text(off, end)}, end
}

// parseAttrs parses attributes up to '>' or '/>'.
func (p *parser) parseAttrs(off int, tag string, tagOff int) ([]ast.Attr, bool, int) {
	var attrs []ast.Attr
	for {
		off = p.skipSpace(off)
		if off >= len(p.src) {
			p.fail(tagOff, "tag <%s> is truncated: expected '>'", tag)
		}
		switch {
		case p.src[off] == '>':
			return attrs, false, off + 1
		case p.at(off, "/>"):
			return attrs, true, off + 2
		case p.src[off] == '{':
			p.fail(off, "expressions are not allowed in place of an attribute name")
		}
		nameStart := off
		for off < len(p.src) {
			c := p.src[off]
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '=' || c == '>' || c == '/' ||
				c == '"' || c == '\'' || c == '<' || c == '{' || c == '}' {
				break
			}
			off++
		}
		if off == nameStart {
			p.fail(off, "unexpected character %q in tag <%s>", p.src[off], tag)
		}
		a := ast.Attr{At: ast.At{P: p.pos(nameStart)}, Name: p.text(nameStart, off), Kind: ast.AttrBool}
		if off < len(p.src) && p.src[off] == '{' {
			p.fail(off, "expressions are not allowed in attribute names")
		}
		next := p.skipSpace(off)
		if next < len(p.src) && p.src[next] == '=' {
			off = p.skipSpace(next + 1)
			if off >= len(p.src) {
				p.fail(nameStart, "attribute %s has no value", a.Name)
			}
			switch q := p.src[off]; q {
			case '"', '\'':
				end := strings.IndexByte(string(p.src[off+1:]), q)
				if end < 0 {
					p.fail(off, "value of attribute %s is not closed", a.Name)
				}
				a.Kind = ast.AttrStatic
				a.Value = p.text(off+1, off+1+end)
				if strings.Contains(a.Value, "{") && strings.Contains(a.Value, "}") && looksLikeInterp(a.Value) {
					p.failUnderline(off, end+2, a.Name+"={…}", "interpolation inside quotes is not supported; use %s={expr} with Go string concatenation", a.Name)
				}
				off += end + 2
			case '{':
				code, end := p.scanGo(off+1, kInterp, false)
				if len(code.Parts) == 0 || strings.TrimSpace(code.String()) == "" {
					p.fail(off, "empty expression in value of attribute %s", a.Name)
				}
				a.Kind = ast.AttrExpr
				a.Expr = code
				off = end + 1
			default:
				p.fail(off, "value of attribute %s must be quoted or {…}", a.Name)
			}
		}
		attrs = append(attrs, a)
	}
}

// looksLikeInterp roughly detects `btn-{size}` in a static value.
func looksLikeInterp(v string) bool {
	i := strings.IndexByte(v, '{')
	j := strings.IndexByte(v[i:], '}')
	if j < 0 {
		return false
	}
	inner := strings.TrimSpace(v[i+1 : i+j])
	if inner == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(inner)
	return scan.IsIdentStart(r) && !strings.ContainsAny(inner, ":;")
}

// parseInterp parses {expr} at off. It returns nil for empty {/* comment */}.
func (p *parser) parseInterp(off int, ctx ast.EscapeCtx) (ast.Node, int) {
	code, end := p.scanGo(off+1, kInterp, false)
	if end >= len(p.src) || p.src[end] != '}' {
		p.fail(off, "interpolation '{' is not closed")
	}
	if len(code.Parts) == 0 || (!code.HasTags() && strings.TrimSpace(stripComments(code.String())) == "") {
		return nil, end + 1
	}
	return &ast.Interp{At: ast.At{P: p.pos(off)}, Code: code, Ctx: ctx}, end + 1
}

func stripComments(s string) string {
	src := []byte(s)
	var b strings.Builder
	for i := 0; i < len(src); {
		j := scan.SkipSpace(src, i)
		if j > i {
			b.WriteByte(' ')
			i = j
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

// parseComponent parses a component call and distributes its children into slots.
func (p *parser) parseComponent(at ast.At, name string, attrs []ast.Attr, selfClose bool, off int) (ast.Node, int) {
	c := &ast.Component{At: at, Name: name, SelfClose: selfClose}
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		c.Pkg, c.Name = name[:i], name[i+1:]
	}
	for _, a := range attrs {
		if strings.ContainsAny(a.Name, ":@") {
			p.fail(a.P.Off, "attribute %s cannot be passed to component <%s>: the name does not map to a Props field", a.Name, name)
		}
	}
	c.Attrs = attrs
	if selfClose {
		return c, off
	}
	children, end := p.parseList(off, childMode)
	end = p.expectCloseTag(name, at.P.Off, end)
	for _, ch := range children {
		slot := "default"
		switch n := ch.(type) {
		case *ast.Element:
			slot, n.Attrs = p.takeSlotAttr(n.Attrs, slot)
		case *ast.Component:
			slot, n.Attrs = p.takeSlotAttr(n.Attrs, slot)
		}
		if c.Slots == nil {
			c.Slots = map[string][]ast.Node{}
		}
		if _, ok := c.Slots[slot]; !ok {
			c.SlotOrder = append(c.SlotOrder, slot)
		}
		c.Slots[slot] = append(c.Slots[slot], ch)
	}
	return c, end
}

func (p *parser) takeSlotAttr(attrs []ast.Attr, def string) (string, []ast.Attr) {
	for i, a := range attrs {
		if a.Name != "slot" {
			continue
		}
		if a.Kind != ast.AttrStatic {
			p.fail(a.P.Off, "slot attribute must be a static string: slot=\"name\"")
		}
		rest := append(append([]ast.Attr{}, attrs[:i]...), attrs[i+1:]...)
		return a.Value, rest
	}
	return def, attrs
}

func (p *parser) parseSlotDef(at ast.At, name string, attrs []ast.Attr, selfClose bool, off int) (ast.Node, int) {
	s := &ast.SlotDef{At: at, Name: "default"}
	for _, a := range attrs {
		if a.Name != "name" || a.Kind != ast.AttrStatic {
			p.fail(a.P.Off, "<slot> accepts only a static name=\"…\" attribute")
		}
		s.Name = a.Value
	}
	if selfClose {
		return s, off
	}
	children, end := p.parseList(off, childMode)
	s.Fallback = children
	return s, p.expectCloseTag(name, at.P.Off, end)
}

// parseRawText parses <style>/<script>: content verbatim up to the closing tag.
func (p *parser) parseRawText(at ast.At, tag string, attrs []ast.Attr, selfClose bool, off int) (ast.Node, int) {
	inline := false
	var kept []ast.Attr
	for _, a := range attrs {
		if a.Name == "is:inline" {
			inline = true
			continue
		}
		kept = append(kept, a)
	}
	if len(kept) > 0 {
		inline = true
	}
	var content string
	contentOff := off
	end := off
	if !selfClose {
		idx := indexFold(p.src[off:], "</"+tag)
		if idx < 0 {
			p.failUnderline(at.P.Off, len(tag)+1, "", "tag <%s> is not closed (opened at %s)", tag, p.posString(at.P.Off))
		}
		content = p.text(off, off+idx)
		cur := p.skipSpace(off + idx + 2 + len(tag))
		if cur >= len(p.src) || p.src[cur] != '>' {
			p.fail(off+idx, "closing tag </%s> has no '>'", tag)
		}
		end = cur + 1
	}

	if tag == "style" {
		if i := findGoExprInRaw(content); i >= 0 {
			p.fail(contentOff+i, "{expr} inside <style> is not allowed")
		}
		s := &ast.StyleBlock{At: at, Attrs: kept, CSS: content, Inline: inline}
		if !inline {
			s.Key = fmt.Sprintf("%s#%d", p.key, p.styleN)
			p.styleN++
		}
		return s, end
	}

	s := &ast.ScriptBlock{At: at, Attrs: kept}
	s.Parts = p.splitScript(content, contentOff)
	dynamic := false
	for _, part := range s.Parts {
		if part.Expr != nil {
			dynamic = true
		}
	}
	s.Inline = inline || dynamic
	if !s.Inline {
		s.Key = fmt.Sprintf("%s#%d", p.key, p.scriptN)
		p.scriptN++
	}
	return s, end
}

func indexFold(b []byte, s string) int {
	n := len(s)
	for i := 0; i+n <= len(b); i++ {
		if strings.EqualFold(string(b[i:i+n]), s) {
			return i
		}
	}
	return -1
}

const jsonPrefix = "__gostro.JSON("

// splitScript extracts {__gostro.JSON(…)} insertions from <script> content.
func (p *parser) splitScript(content string, base int) []ast.RawPart {
	var parts []ast.RawPart
	last := 0
	for i := 0; i < len(content); i++ {
		if content[i] != '{' {
			continue
		}
		j := i + 1
		for j < len(content) && (content[j] == ' ' || content[j] == '\t') {
			j++
		}
		if strings.HasPrefix(content[j:], jsonPrefix) {
			if i > last {
				parts = append(parts, ast.RawPart{At: ast.At{P: p.pos(base + last)}, Text: content[last:i]})
			}
			code, end := p.scanGo(base+i+1, kInterp, false)
			if end >= len(p.src) || p.src[end] != '}' {
				p.fail(base+i, "interpolation '{' is not closed")
			}
			parts = append(parts, ast.RawPart{At: ast.At{P: p.pos(base + i)}, Expr: &code})
			last = end + 1 - base
			i = last - 1
			continue
		}
		if isGoExprAt(content[i:]) {
			p.failUnderline(base+i, exprLen(content[i:]), "{__gostro.JSON(v)}", "{expr} inside <script> is not allowed; use {__gostro.JSON(v)}")
		}
	}
	if last < len(content) {
		parts = append(parts, ast.RawPart{At: ast.At{P: p.pos(base + last)}, Text: content[last:]})
	}
	return parts
}

// goExprPrefixes are insertion prefixes that look like a Go expression rather than JS/CSS.
var goExprPrefixes = []string{"props.", "ctx.", "slots.", "__gostro."}

// isGoExprAt reports whether s ('{' at s[0]) starts with a Go insertion such as {props.X}.
// Ordinary JS/CSS blocks ({ color: red }, {console.log(x)}) are left alone.
func isGoExprAt(s string) bool {
	rest := strings.TrimLeft(s[1:], " \t")
	for _, pre := range goExprPrefixes {
		if strings.HasPrefix(rest, pre) {
			return true
		}
	}
	return false
}

// findGoExprInRaw looks for a Go insertion in <style>/<script> content; it returns the offset of '{' or -1.
func findGoExprInRaw(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '{' && isGoExprAt(s[i:]) {
			return i
		}
	}
	return -1
}

func exprLen(s string) int {
	if i := strings.IndexByte(s, '}'); i >= 0 {
		return i + 1
	}
	return 1
}
