package architecture_test

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestCoreDependenciesPointInward(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(file))
	const prefix = "github.com/ninjashadowboy/portfolio-backend-go/internal/"
	for layer, allowed := range map[string][]string{
		"domain":  {},
		"dto":     {"domain"},
		"ports":   {"domain", "dto"},
		"usecase": {"domain", "dto", "ports"},
	} {
		files, err := filepath.Glob(filepath.Join(root, layer, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) == 0 {
			t.Fatalf("missing layer %s", layer)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, spec := range parsed.Imports {
				path, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(path, prefix) {
					if !slices.Contains(allowed, strings.TrimPrefix(path, prefix)) {
						t.Errorf("%s imports outer layer %s", file, path)
					}
				} else if strings.Contains(strings.Split(path, "/")[0], ".") || path == "database/sql" || path == "net/http" {
					t.Errorf("%s depends on infrastructure %s", file, path)
				}
			}
		}
	}
}
