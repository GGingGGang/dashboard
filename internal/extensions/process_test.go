package extensions

import (
	"context"
	"encoding/json"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"idp-dashboard/internal/application"
	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/storage"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "provider.exe")
	sdkRoot, err := filepath.Abs("../../sdk")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile("testdata/provider/main.go")
	if err != nil {
		t.Fatal(err)
	}
	module := t.TempDir()
	if err = os.WriteFile(filepath.Join(module, "main.go"), source, 0600); err != nil {
		t.Fatal(err)
	}
	mod := "module external-fixture\n\ngo 1.25.0\nrequire github.com/GGingGGang/dashboard/sdk v0.0.0\nreplace github.com/GGingGGang/dashboard/sdk => " + filepath.ToSlash(sdkRoot) + "\n"
	if err = os.WriteFile(filepath.Join(module, "go.mod"), []byte(mod), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = module
	cmd.Env = append(os.Environ(), "GOPROXY=off", "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("SDK-only executable: %s %v", output, err)
	}
	return out
}
func info() sdk.ProviderInfo {
	return sdk.ProviderInfo{Kind: "external", Name: "External", Capabilities: []string{"data"}, OnDemand: true, DefaultAuth: "none", AuthMethods: []string{"none"}}
}
func TestProcessPublicationUpdateDeleteAndCancellation(t *testing.T) {
	path := fixture(t)
	p, err := launch(path, info(), sdk.Connection{ID: "one", Kind: "external"}, "private-test-secret")
	if err != nil {
		t.Fatal(err)
	}
	scope := platform.NewSources(context.Background())
	defer scope.Close()
	if err = scope.Register(context.Background(), sdk.SourceDescriptor{ID: "default", Name: "Default", Capabilities: []string{"data"}}, p); err != nil {
		t.Fatal(err)
	}
	if err = p.(sdk.SourcePublisher).PublishSources(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	if len(scope.List()) != 2 {
		t.Fatal("source publication missing")
	}
	read := func(id, action string) (sdk.DataResult, error) {
		ctx, provider, _, release, e := scope.Acquire(context.Background(), id)
		if e != nil {
			return sdk.DataResult{}, e
		}
		defer release()
		return sdk.Resolve(provider, "data").(sdk.DataSource).ReadData(ctx, map[string]any{"action": action})
	}
	if r, e := read("extra", ""); e != nil || r.Value != "old" {
		t.Fatal(r, e)
	}
	if _, e := read("default", "update"); e != nil {
		t.Fatal(e)
	}
	if r, e := read("extra", ""); e != nil || r.Value != "new" {
		t.Fatal(r, e)
	}
	if _, e := read("default", "secret"); e == nil || strings.Contains(e.Error(), "private-test-secret") {
		t.Fatal("secret leaked", e)
	}
	if _, e := read("default", "delete"); e != nil {
		t.Fatal(e)
	}
	if len(scope.List()) != 1 {
		t.Fatal("source deletion missing")
	}
	if sdk.Resolve(p, "builds") != nil {
		t.Fatal("undeclared capability exposed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = sdk.Resolve(p, "data").(sdk.DataSource).ReadData(ctx, map[string]any{"action": "slow"})
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatal("hung provider did not cancel", err)
	}
	if p.(sdk.ProviderStatus).Err() == nil {
		t.Fatal("cancelled process still healthy")
	}
}
func TestProcessRejectsMismatchedMetadataAndCrash(t *testing.T) {
	path := fixture(t)
	wrong := info()
	wrong.Capabilities = []string{"builds"}
	if p, err := launch(path, wrong, sdk.Connection{Kind: "external"}, ""); err == nil {
		_ = sdk.CloseProvider(p)
		t.Fatal("mismatch accepted")
	}
	p, err := launch(path, info(), sdk.Connection{Kind: "external"}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer sdk.CloseProvider(p)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err = sdk.Resolve(p, "data").(sdk.DataSource).ReadData(ctx, map[string]any{"action": "crash"}); err == nil {
		t.Fatal("crash accepted")
	}
}

type credentials struct{}

func (credentials) Get(string) (string, error) { return "", nil }
func (credentials) Set(string, string) error   { return nil }
func (credentials) Delete(string) error        { return nil }
func TestInstalledProcessRecoversThroughApplicationAfterCrash(t *testing.T) {
	exe := fixture(t)
	binary, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	manifest := packageManifest()
	manifest.Cards[0].ConfigSchema = json.RawMessage(`{"type":"object","properties":{"action":{"type":"string"}}}`)
	root := t.TempDir()
	manager := NewManager(root, nil)
	if _, err = manager.Install(packageZIP(t, manifest, map[string][]byte{"provider.exe": binary}), true); err != nil {
		t.Fatal(err)
	}
	registry, _ := platform.NewRegistry()
	if err = NewManager(root, nil).Load(registry); err != nil {
		t.Fatal(err)
	}
	store, err := storage.OpenStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(context.Background(), store, credentials{}, registry, false)
	defer app.Close()
	id, err := app.SaveConnection(application.ConnectionInput{Connection: sdk.Connection{Kind: "external", Name: "External", URL: "http://example.invalid", Auth: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	request := application.ContributionRequest{PluginID: "external", Kind: "card", ID: "value", Input: json.RawMessage(`{"action":"crash"}`)}
	if _, err = app.ExecuteContribution(id, "default", request); err == nil {
		t.Fatal("crash not reported")
	}
	request.Input = json.RawMessage(`{}`)
	result, err := app.ExecuteContribution(id, "default", request)
	if err != nil || result.Value != "root" {
		t.Fatal("process did not recover", result, err)
	}
	if err = app.SetPluginEnabled("external", false); err != nil {
		t.Fatal(err)
	}
	if _, err = app.ExecuteContribution(id, "default", request); err == nil {
		t.Fatal("disabled process queried")
	}
	if err = app.SetPluginEnabled("external", true); err != nil {
		t.Fatal(err)
	}
	result, err = app.ExecuteContribution(id, "default", request)
	if err != nil || result.Value != "root" {
		t.Fatal("reactivation failed", result, err)
	}
}
