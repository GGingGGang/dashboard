package httpapi_test

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/providers/prometheus"
)

func connection(kind, address string) platform.Connection {
	return platform.Connection{ID: "test", Kind: kind, Name: "test", URL: address, Auth: "none"}
}
func testProvider(c platform.Connection, secret string) (platform.Provider, error) {
	r, err := platform.NewRegistry(prometheus.Definition())
	if err != nil {
		return nil, err
	}
	return r.New(c, secret)
}
func TestAuthAndRedirectDoNotLeak(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect was followed") }))
	defer destination.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 302) }))
	defer srv.Close()
	c := connection("prometheus", srv.URL)
	c.Auth = "bearer"
	p, _ := testProvider(c, "top-secret")
	if e := p.Check(context.Background()); e == nil || strings.Contains(e.Error(), "top-secret") {
		t.Fatalf("unsafe error: %v", e)
	}
	for _, raw := range []string{"file:///etc/passwd", "https://user:pass@example.org", "https://example.org?token=secret"} {
		c.URL = raw
		if _, e := testProvider(c, "secret"); e == nil {
			t.Error("accepted invalid URL", raw)
		}
	}
}
func TestTLSCustomCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"scalar","result":[1700000000,"1"]}}`)
	}))
	defer srv.Close()
	p, _ := testProvider(connection("prometheus", srv.URL), "")
	if e := p.Check(context.Background()); e == nil {
		t.Error("untrusted certificate accepted")
	}
	ca := filepath.Join(t.TempDir(), "test-ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	c := connection("prometheus", srv.URL)
	c.CAFile = ca
	trusted, err := testProvider(c, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := trusted.Check(context.Background()); err != nil {
		t.Fatalf("explicit test CA was not trusted: %v", err)
	}
}
