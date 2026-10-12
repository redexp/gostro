// Package ast defines the syntax tree nodes of a .gostro file.
package ast

import "github.com/redexp/gostro/internal/token"

// Pos is a position in a .gostro file.
type Pos = token.Pos

// Node is a node in statement position (component body or element children).
type Node interface{ Pos() Pos }

// At is embedded in nodes and implements Pos().
type At struct{ P Pos }

func (a At) Pos() Pos { return a.P }

// CodePart is a piece of Go code: either verbatim text or a tag in expression position.
type CodePart struct {
	Text string
	At   Pos  // position of the first byte of Text or of the tag
	Tag  Node // *Element or *Component; if non-nil, Text is empty
}

// Code is Go code passed through verbatim, possibly containing tag expressions.
type Code struct {
	Parts []CodePart
}

// HasTags reports whether the code contains tag expressions.
func (c Code) HasTags() bool {
	for _, p := range c.Parts {
		if p.Tag != nil {
			return true
		}
	}
	return false
}

// String returns the code with tags replaced by "<…>", for diagnostics.
func (c Code) String() string {
	s := ""
	for _, p := range c.Parts {
		if p.Tag != nil {
			s += "<…>"
		} else {
			s += p.Text
		}
	}
	return s
}

// GoChunk is a Go statement passed through verbatim.
type GoChunk struct {
	At
	Code Code
}

// AttrKind is the form of an attribute.
type AttrKind int

const (
	AttrStatic AttrKind = iota // name="…"
	AttrExpr                   // name={…}
	AttrBool                   // name
)

// Attr is an element or component attribute.
type Attr struct {
	At
	Name  string
	Kind  AttrKind
	Value string // for AttrStatic: the quoted content verbatim
	Expr  Code   // for AttrExpr
}

// Element is an HTML element.
type Element struct {
	At
	Name      string
	Attrs     []Attr
	Children  []Node
	SelfClose bool
	Void      bool
	SetHTML   *Code // set:html={…}
}

// Component is a component call <Card …> or <ui.Card …>.
type Component struct {
	At
	Pkg, Name string
	Attrs     []Attr
	Slots     map[string][]Node
	SlotOrder []string // slot names in order of first appearance
	SelfClose bool
}

// EscapeCtx is the escaping context of an interpolation.
type EscapeCtx int

const (
	CtxText   EscapeCtx = iota // element child
	CtxScript                  // inside <script>: __gostro.JSON only
)

// Interp is an {expr} interpolation.
type Interp struct {
	At
	Code Code
	Ctx  EscapeCtx
}

// Raw is static markup emitted verbatim (<!DOCTYPE …>, <!-- … -->).
type Raw struct {
	At
	Text string
}

// Control is an if / for / bare block { … } with children.
// For if: Else holds either a single *Control (else if, ElseIf=true) or the else-branch children.
type Control struct {
	At
	Keyword string // "if", "for", or "" for a block
	Clause  Code   // from the keyword up to '{', exclusive
	Body    []Node
	HasElse bool
	ElseIf  bool
	ElseAt  Pos
	Else    []Node
}

// Switch is a switch / select with case clauses.
type Switch struct {
	At
	Keyword string
	Clause  Code // from the keyword up to '{', exclusive
	Cases   []SwitchCase
}

// SwitchCase is a single `case …:` or `default:` clause.
type SwitchCase struct {
	At
	Clause Code // from case/default up to ':', inclusive
	Body   []Node
}

// SlotDef is a slot definition <slot name="…">fallback</slot>.
type SlotDef struct {
	At
	Name     string
	Fallback []Node
}

// RawPart is a part of <script>/<style> content: text or {__gostro.JSON(…)}.
type RawPart struct {
	At
	Text string
	Expr *Code
}

// StyleBlock is a <style> block.
type StyleBlock struct {
	At
	Attrs  []Attr
	CSS    string
	Inline bool
	Key    string
}

// ScriptBlock is a <script> block.
type ScriptBlock struct {
	At
	Attrs  []Attr
	Parts  []RawPart
	Inline bool
	Key    string
}

// JS returns the content of a static script.
func (s *ScriptBlock) JS() string {
	out := ""
	for _, p := range s.Parts {
		out += p.Text
	}
	return out
}

// Import is an import declaration verbatim (without the keyword for single imports).
type Import struct {
	At
	Text string // `import "strings"` or `import ( … )`
}

// Func is a top-level function.
type Func struct {
	At
	Name      string
	Component bool
	Sig       Code // component: from 'func' up to the parameters' ')', exclusive; otherwise up to the body '{'
	Body      []Node
	AddSlots  bool // add a slots __gostro.Slots parameter
}

// Props is a top-level `type XxxProps struct{…}` (hoisted to package level).
type Props struct {
	At
	Name string // XxxProps
	Type Code   // `struct { … }`
}

// File is a parsed .gostro file.
type File struct {
	Name       string // base file name (card.gostro)
	Key        string // key for registering styles/scripts
	Package    string // from the package directive, or ""
	PackageAt  Pos
	Imports    []Import
	PropsTypes []*Props // type XxxProps struct{…}
	Funcs      []*Func
	Body       []Node // implicit component body
	BodyAt     Pos
	HasBody    bool
}
