package extensions

import (
	"archive/zip"
	"encoding/json"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"idp-dashboard/internal/platform"
	"os"
	"path/filepath"
	"testing"
)

func packageZIP(t *testing.T, manifest sdk.Manifest, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plugin.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(f)
	raw, _ := json.Marshal(manifest)
	all := map[string][]byte{"manifest.json": raw}
	for key, value := range entries {
		all[key] = value
	}
	for name, data := range all {
		w, e := writer.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write(data); e != nil {
			t.Fatal(e)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
func packageManifest() sdk.Manifest {
	return sdk.Manifest{ID: "external", Version: "1.0", ContractVersion: 1, Providers: []sdk.ProviderContribution{{ID: "external", Name: "External", Executable: "provider.exe", Info: func() *sdk.ProviderInfo { i := info(); return &i }()}}, Cards: []sdk.CardContribution{{ID: "value", Title: "Value", Capability: "data", Renderer: "scalar"}}}
}
func TestPackageInstallUpgradeRemovalRequireRestart(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(root, nil)
	m := packageManifest()
	path := packageZIP(t, m, map[string][]byte{"provider.exe": []byte("not executed at install")})
	if _, err := manager.Install(path, false); err == nil {
		t.Fatal("trust required")
	}
	if _, err := manager.Install(path, true); err != nil {
		t.Fatal(err)
	}
	state, err := manager.List()
	if err != nil || len(state) != 1 || !state[0].PendingRestart {
		t.Fatal(state, err)
	}
	registry, _ := platform.NewRegistry()
	running := NewManager(root, nil)
	if err = running.Load(registry); err != nil || len(registry.Plugins()) != 1 {
		t.Fatal(err)
	}
	m.Version = "2.0"
	upgrade := packageZIP(t, m, map[string][]byte{"provider.exe": []byte("new code")})
	if _, err = running.Install(upgrade, true); err != nil {
		t.Fatal(err)
	}
	if registry.Plugins()[0].Manifest.Version != "1.0" {
		t.Fatal("running manifest changed")
	}
	state, _ = running.List()
	if state[0].Manifest.Version != "2.0" || !state[0].PendingRestart {
		t.Fatal(state)
	}
	next, _ := platform.NewRegistry()
	restarted := NewManager(root, nil)
	if err = restarted.Load(next); err != nil {
		t.Fatal(err)
	}
	if next.Plugins()[0].Manifest.Version != "2.0" {
		t.Fatal("upgrade not loaded")
	}
	if err = restarted.Uninstall("external"); err != nil {
		t.Fatal(err)
	}
	state, _ = restarted.List()
	if !state[0].PendingRemoval || len(next.Plugins()) != 1 {
		t.Fatal("running package removed")
	}
	empty, _ := platform.NewRegistry()
	if err = NewManager(root, nil).Load(empty); err != nil || len(empty.Plugins()) != 0 {
		t.Fatal("uninstall not applied", err)
	}
}
func TestRejectUnsafeOrInvalidPackagesWithoutChangingIndex(t *testing.T) {
	m := packageManifest()
	manager := NewManager(t.TempDir(), nil)
	for _, name := range []string{"../escape", "/absolute", "C:/escape", `dir\escape`, "CON", "dir/../escape", "provider.exe/child", "Manifest.json"} {
		t.Run(name, func(t *testing.T) {
			path := packageZIP(t, m, map[string][]byte{"provider.exe": []byte("exe"), name: []byte("bad")})
			if _, err := manager.Install(path, true); err == nil {
				t.Fatal("unsafe ZIP accepted")
			}
			state, err := manager.List()
			if err != nil || len(state) != 0 {
				t.Fatal("index mutated", state, err)
			}
		})
	}
	m.ContractVersion = 999
	if _, err := manager.Install(packageZIP(t, m, map[string][]byte{"provider.exe": []byte("exe")}), true); err == nil {
		t.Fatal("incompatible manifest accepted")
	}
}
func TestBuiltinProtectionAndBrokenIndexPreserved(t *testing.T) {
	root := t.TempDir()
	m := packageManifest()
	manager := NewManager(root, []platform.PluginState{{Manifest: m}})
	if _, err := manager.Install(packageZIP(t, m, map[string][]byte{"provider.exe": []byte("exe")}), true); err == nil {
		t.Fatal("builtin replaced")
	}
	raw := []byte(`broken original`)
	path := filepath.Join(root, "index.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Uninstall("another"); err == nil {
		t.Fatal("broken index overwritten")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(raw) {
		t.Fatal("index lost")
	}
}
