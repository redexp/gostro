package gen

import (
	"bytes"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/redexp/gostro/internal/parse"
)

// OutputPath returns the path of the generated file: card.gostro → card_gostro.go.
func OutputPath(path string) string {
	return strings.TrimSuffix(path, ".gostro") + "_gostro.go"
}

// Result is the result of generating a single file.
type Result struct {
	Out     string // path of the generated file
	Changed bool   // the file was rewritten
}

// GenerateFile transpiles a .gostro file and writes the result if it changed.
func GenerateFile(path string) (Result, error) {
	res := Result{Out: OutputPath(path)}
	src, err := os.ReadFile(path)
	if err != nil {
		return res, err
	}
	out, err := Source(path, src)
	if err != nil {
		return res, err
	}
	if old, err := os.ReadFile(res.Out); err == nil && bytes.Equal(old, out) {
		return res, nil
	}
	if err := os.WriteFile(res.Out, out, 0o644); err != nil {
		return res, err
	}
	res.Changed = true
	return res, nil
}

// Source transpiles the content of file path (the file on disk is not read,
// but its directory is used to determine the package and the style key).
func Source(path string, src []byte) ([]byte, error) {
	f, err := parse.ParseFile(filepath.Base(path), StyleKey(path), src)
	if err != nil {
		return nil, err
	}
	pkg := f.Package
	if pkg == "" {
		pkg = PackageName(filepath.Dir(path))
	}
	return Generate(f, pkg)
}

// StyleKey returns the style and script registration key: the path relative to
// the module root (the directory with go.mod) with forward slashes; outside a
// module, the file name.
func StyleKey(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Base(path)
	}
	for dir := filepath.Dir(abs); ; {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			rel, err := filepath.Rel(dir, abs)
			if err != nil {
				break
			}
			return filepath.ToSlash(rel)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Base(path)
}

// PackageName determines the package for a directory: the package of sibling
// .go files, otherwise the directory name normalized to a Go identifier.
func PackageName(dir string) string {
	entries, _ := os.ReadDir(dir)
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_gostro.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.PackageClauseOnly)
		if err == nil && f.Name != nil {
			return f.Name.Name
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return sanitizeIdent(strings.ToLower(filepath.Base(abs)))
}

func sanitizeIdent(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" || unicode.IsDigit([]rune(out)[0]) {
		out = "p" + out
	}
	return out
}
