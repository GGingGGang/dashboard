package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
	"idp-dashboard/internal/demo"
	"idp-dashboard/internal/platform"
)

// Opt in: creates and removes only credentials belonging to this test.
func TestWindowsCredentialLifecycle(t *testing.T) {
	if os.Getenv("IDP_TEST_KEYRING") != "1" {
		t.Skip("set IDP_TEST_KEYRING=1 to test the current user's credential store")
	}
	base, closeDemo := demo.Start()
	defer closeDemo()
	a := NewApp(false)
	a.ctx = context.Background()
	var err error
	a.store, err = platform.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.shutdown(a.ctx)
	c := platform.Connection{Kind: "prometheus", Name: "Credential lifecycle test", URL: base + "/prometheus", Auth: "bearer"}
	secret := "test-only-" + randomID()
	id, err := a.SaveConnection(ConnectionInput{Connection: c, Secret: secret})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := a.find(id)
	if err != nil {
		t.Fatal(err)
	}
	defer keyring.Delete("IDPDashboard", saved.SecretRef)
	got, err := credential(saved)
	if err != nil || got != secret {
		t.Fatal("credential round trip failed")
	}
	if _, err = a.TestConnection(ConnectionInput{Connection: saved.Connection}); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "backup.db")
	if err = a.store.Backup(backup); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(backup)
	if err != nil || strings.Contains(string(data), secret) {
		t.Fatal("database contains a secret or cannot be read")
	}
	if err = a.DeleteConnection(id); err != nil {
		t.Fatal(err)
	}
	if _, err = keyring.Get("IDPDashboard", saved.SecretRef); err != keyring.ErrNotFound {
		t.Fatal("credential was not removed")
	}
}

func TestChangedAuthenticationRequiresNewSecret(t *testing.T) {
	a := NewApp(false)
	a.ctx = context.Background()
	var err error
	a.store, err = platform.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer a.store.Close()
	legacy := platform.Connection{ID: "legacy", Kind: "argocd", Name: "Argo", URL: "http://localhost", Auth: "basic", Username: "admin"}
	if err = a.store.SaveConnection(legacy, "unused-test-reference"); err != nil {
		t.Fatal(err)
	}
	changed := legacy
	changed.Auth = "argocd-login"
	if _, err = a.TestConnection(ConnectionInput{Connection: changed}); err == nil || !strings.Contains(err.Error(), "새 토큰 또는 비밀번호") {
		t.Fatal("test reused a credential across authentication methods")
	}
	if _, err = a.SaveConnection(ConnectionInput{Connection: changed}); err == nil || !strings.Contains(err.Error(), "새 토큰 또는 비밀번호") {
		t.Fatal("save reused a credential across authentication methods")
	}
}
