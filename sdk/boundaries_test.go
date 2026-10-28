package sdk_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The SDK must build without the application, including its test dependencies.
func TestSDKDoesNotImportCore(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			dep, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			own := dep == "github.com/GGingGGang/dashboard/sdk" || strings.HasPrefix(dep, "github.com/GGingGGang/dashboard/sdk/")
			standard := !strings.Contains(strings.Split(dep, "/")[0], ".") && dep != "C" && dep != "idp-dashboard" && !strings.HasPrefix(dep, "idp-dashboard/")
			if !own && !standard {
				t.Errorf("%s imports %s; SDK may depend only on itself and the standard library", path, dep)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
