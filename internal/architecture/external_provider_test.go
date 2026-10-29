package architecture_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Compile the actual shipped adapters in a consumer module which has no
// requirement or replacement for the application. This also detects transitive
// dependencies that an import-only boundary check would miss.
func TestProvidersBuildOutsideCore(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	consumer := t.TempDir()
	mod := fmt.Sprintf(`module example.com/provider-consumer

go 1.25.0
require (
 github.com/GGingGGang/dashboard/sdk v0.0.0
 github.com/itchyny/gojq v0.12.17
 github.com/itchyny/timefmt-go v0.1.6
)
replace github.com/GGingGGang/dashboard/sdk => %q
`, filepath.ToSlash(filepath.Join(root, "sdk")))
	if err := os.WriteFile(filepath.Join(consumer, "go.mod"), []byte(mod), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "internal", "providers"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "internal" {
			continue
		}
		source := filepath.Join(root, "internal", "providers", entry.Name())
		files, err := os.ReadDir(source)
		if err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(consumer, entry.Name())
		if err := os.Mkdir(dest, 0700); err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") || strings.HasSuffix(file.Name(), "_test.go") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(source, file.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dest, file.Name()), body, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	sum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(consumer, "go.sum"), sum, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = consumer
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("external provider build: %v\n%s", err, out)
	}
}
