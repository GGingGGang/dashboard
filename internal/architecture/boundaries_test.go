package architecture_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Check direct production imports; integration tests may compose several layers.
func TestPackageBoundaries(t *testing.T) {
	const prefix = "idp-dashboard/internal/"
	root := filepath.Join("..", "..", "internal")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := filepath.ToSlash(rel)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			dep, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			local := dep == "idp-dashboard" || strings.HasPrefix(dep, "idp-dashboard/")
			standard := !local && !strings.Contains(strings.Split(dep, "/")[0], ".") && dep != "C"
			allowed := true
			switch {
			case pkg == "platform" || strings.HasPrefix(pkg, "platform/"):
				allowed = standard
			case pkg == "collector" || strings.HasPrefix(pkg, "collector/"):
				allowed = standard || dep == prefix+"platform"
			case pkg == "storage" || strings.HasPrefix(pkg, "storage/"):
				allowed = !local || dep == prefix+"platform"
			case pkg == "providers":
				allowed = !local || dep == prefix+"platform" || strings.HasPrefix(dep, prefix+"providers/")
			case strings.HasPrefix(pkg, "providers/"):
				allowed = !local || dep == prefix+"platform" || dep == prefix+"providers/internal/httpapi"
			}
			if !allowed {
				t.Errorf("%s imports %s across its package boundary; see ARCHITECTURE.md", pkg, dep)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
