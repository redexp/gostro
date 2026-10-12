// Command gostro transpiles .gostro files to Go.
//
//	gostro generate [paths...]     transpile (default ./...)
//	gostro fmt [paths...]          format .gostro files (not implemented yet)
//	gostro watch [paths...]        watch for changes, regenerate, restart the app and reload the page
//	gostro version
//
// //go:generate gostro generate is supported.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/redexp/gostro/internal/gen"
	"github.com/redexp/gostro/internal/parse"
)

// Version is the gostro version.
const Version = "0.1.0"

const usage = `gostro: HTML templates for Go with markup literals.

Usage:
  gostro generate [-v] [-w] [-keep-going] [paths...]
  gostro watch [-v] [-cmd "go run ."] [-sse :7331] [paths...]
  gostro fmt [paths...]
  gostro version

Paths: file.gostro, dir (without subdirectories) or dir/... (recursive).
Default: ./...
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "generate", "gen":
		err = runGenerate(args)
	case "watch":
		err = runWatch(args)
	case "fmt":
		err = errors.New("gostro fmt is not implemented yet (stage 9 of the spec)")
	case "version", "-version", "--version":
		fmt.Println("gostro", Version)
	case "help", "-h", "-help", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "gostro: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		if !errors.Is(err, errReported) {
			fmt.Fprintln(os.Stderr, "gostro:", err)
		}
		os.Exit(1)
	}
}

// errReported means the errors have already been reported.
var errReported = errors.New("generation errors")

func runGenerate(args []string) error {
	fl := flag.NewFlagSet("generate", flag.ExitOnError)
	verbose := fl.Bool("v", false, "verbose output")
	watch := fl.Bool("w", false, "watch for changes (like gostro watch without -cmd)")
	keepGoing := fl.Bool("keep-going", false, "do not stop at the first error")
	fl.Parse(args)

	if *watch {
		return watchLoop(watchOptions{patterns: fl.Args(), verbose: *verbose})
	}
	files, err := findFiles(fl.Args())
	if err != nil {
		return err
	}
	_, err = generateAll(files, *verbose, *keepGoing)
	return err
}

// generateAll transpiles the files and returns the number of changed ones.
func generateAll(files []string, verbose, keepGoing bool) (int, error) {
	changed, failed := 0, 0
	for _, f := range files {
		res, err := gen.GenerateFile(f)
		if err != nil {
			printError(err)
			failed++
			if !keepGoing {
				return changed, errReported
			}
			continue
		}
		if res.Changed {
			changed++
		}
		if verbose {
			state := "unchanged"
			if res.Changed {
				state = "written"
			}
			fmt.Fprintf(os.Stderr, "%s → %s (%s)\n", f, res.Out, state)
		}
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "files: %d, changed: %d, errors: %d\n", len(files), changed, failed)
	}
	if failed > 0 {
		return changed, errReported
	}
	return changed, nil
}

func printError(err error) {
	var pe *parse.Error
	if errors.As(err, &pe) {
		fmt.Fprintln(os.Stderr, pe.Pretty())
		return
	}
	fmt.Fprintln(os.Stderr, err)
}

// findFiles expands paths into a sorted list of .gostro files.
func findFiles(patterns []string) ([]string, error) {
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	seen := map[string]bool{}
	var files []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			files = append(files, p)
		}
	}
	for _, pat := range patterns {
		if dir, ok := strings.CutSuffix(pat, "..."); ok {
			dir = strings.TrimSuffix(dir, "/")
			if dir == "" || dir == "." {
				dir = "."
			}
			err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					if path != dir && skipDir(d.Name()) {
						return filepath.SkipDir
					}
					return nil
				}
				if strings.HasSuffix(path, ".gostro") {
					add(path)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		info, err := os.Stat(pat)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			add(pat)
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(pat, "*.gostro"))
		for _, m := range matches {
			add(m)
		}
	}
	sort.Strings(files)
	return files, nil
}

// skipDir reports directories that Go tools ignore for ./...
func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" || name == "node_modules"
}
