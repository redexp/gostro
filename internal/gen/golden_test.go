package gen_test

import (
	"bytes"
	"errors"
	"flag"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/redexp/gostro/internal/gen"
	"github.com/redexp/gostro/internal/parse"
)

var update = flag.Bool("update", false, "rewrite expected files")

const root = "../.."

// TestGolden: testdata/golden/x.gostro → x.go.want (compared after gofmt).
func TestGolden(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(root, "testdata", "golden", "*.gostro"))
	if len(files) == 0 {
		t.Fatal("no golden files")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			got, err := gen.Source(f, src)
			if err != nil {
				t.Fatal(err)
			}
			wantPath := strings.TrimSuffix(f, ".gostro") + ".go.want"
			if *update {
				os.WriteFile(wantPath, got, 0o644)
				return
			}
			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("%v (run with -update)", err)
			}
			if !bytes.Equal(gofmt(t, got), gofmt(t, want)) {
				t.Errorf("output differs from %s:\n%s", wantPath, got)
			}
			// Determinism: a second run produces byte-for-byte identical output.
			again, _ := gen.Source(f, src)
			if !bytes.Equal(got, again) {
				t.Error("generation is not deterministic")
			}
		})
	}
}

func gofmt(t *testing.T, src []byte) []byte {
	t.Helper()
	out, err := format.Source(src)
	if err != nil {
		t.Fatalf("gofmt: %v", err)
	}
	return out
}

// TestGoldenCompiles: every golden output actually compiles.
func TestGoldenCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("compilation skipped in -short mode")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not found")
	}
	absRoot, _ := filepath.Abs(root)
	dir := t.TempDir()
	gomod := "module goldtest\n\ngo 1.22\n\nrequire github.com/redexp/gostro v0.0.0\n\nreplace github.com/redexp/gostro => " + filepath.ToSlash(absRoot) + "\n"
	must(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644))
	if sum, err := os.ReadFile(filepath.Join(absRoot, "go.sum")); err == nil {
		must(t, os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0o644))
	}
	files, _ := filepath.Glob(filepath.Join(root, "testdata", "golden", "*.gostro"))
	for _, f := range files {
		src, _ := os.ReadFile(f)
		out, err := gen.Source(f, src)
		if err != nil {
			t.Fatal(err)
		}
		pkgDir := filepath.Join(dir, strings.TrimSuffix(filepath.Base(f), ".gostro"))
		must(t, os.MkdirAll(pkgDir, 0o755))
		must(t, os.WriteFile(filepath.Join(pkgDir, gen.OutputPath(filepath.Base(f))), out, 0o644))
	}
	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go vet: %v\n%s", err, out)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// TestErrors: testdata/errors/x.gostro → error text in x.err.
func TestErrors(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(root, "testdata", "errors", "*.gostro"))
	if len(files) == 0 {
		t.Fatal("no error files")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, _ := os.ReadFile(f)
			_, err := gen.Source(f, src)
			if err == nil {
				t.Fatal("expected an error")
			}
			var pe *parse.Error
			if !errors.As(err, &pe) {
				t.Fatalf("error is not *parse.Error: %v", err)
			}
			got := pe.Pretty() + "\n"
			errPath := strings.TrimSuffix(f, ".gostro") + ".err"
			if *update {
				os.WriteFile(errPath, []byte(got), 0o644)
				return
			}
			want, err := os.ReadFile(errPath)
			if err != nil {
				t.Fatalf("%v (run with -update)", err)
			}
			if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
				t.Errorf("\n got: %s\nwant: %s", got, want)
			}
		})
	}
}
