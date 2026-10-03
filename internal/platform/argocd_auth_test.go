package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestArgoSessionLoginAndExpiry(t *testing.T) {
	logins, reads := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prefix/api/v1/session":
			logins++
			if r.Method != "POST" || r.Header.Get("Authorization") != "" || r.Header.Get("Content-Type") != "application/json" {
				t.Error("invalid login request")
			}
			var input map[string]string
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input["username"] != "reader" || input["password"] != "local-password" {
				t.Error("invalid login payload")
			}
			fmt.Fprintf(w, `{"token":"session-%d"}`, logins)
		case "/prefix/api/v1/applications":
			reads++
			if r.Method != "GET" || r.Header.Get("Authorization") != fmt.Sprintf("Bearer session-%d", logins) {
				t.Error("missing session bearer")
			}
			if reads == 2 {
				w.WriteHeader(401)
				return
			}
			fmt.Fprint(w, `{"items":[{"metadata":{"name":"payments","namespace":"argocd"},"status":{"sync":{"status":"Synced"},"health":{"status":"Healthy"}}}]}`)
		default:
			t.Error("unexpected endpoint")
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := connection("argocd", srv.URL+"/prefix")
	c.Auth, c.Username = "argocd-login", "reader"
	p, err := New(c, "local-password")
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		apps, err := p.(CD).Deployments(context.Background())
		if err != nil || len(apps) != 1 {
			t.Fatalf("read failed: %v", err)
		}
	}
	if logins != 2 || reads != 4 {
		t.Fatalf("login=%d read=%d; session cache/refresh failed", logins, reads)
	}
}

func TestArgoRejectedPasswordDoesNotLoopOrLeak(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(401)
		fmt.Fprint(w, `{"error":"test-password"}`)
	}))
	defer srv.Close()
	c := connection("argocd", srv.URL)
	c.Auth, c.Username = "argocd-login", "reader"
	p, err := New(c, "test-password")
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err = p.Check(context.Background()); err == nil || strings.Contains(err.Error(), "test-password") {
			t.Fatal("missing or unsafe login error")
		}
	}
	if requests != 1 {
		t.Fatalf("repeated a rejected login %d times", requests)
	}
}

func TestArgoBearerDoesNotLoginAndBasicRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/applications" || r.Method != "GET" || r.Header.Get("Authorization") != "Bearer api-token" {
			t.Error("unexpected request")
		}
		w.WriteHeader(403)
	}))
	defer srv.Close()
	c := connection("argocd", srv.URL)
	c.Auth = "bearer"
	p, err := New(c, "api-token")
	if err != nil {
		t.Fatal(err)
	}
	if err = p.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "권한") {
		t.Fatal("missing permissions explanation")
	}
	c.Auth, c.Username = "basic", "admin"
	if _, err = New(c, "password"); err == nil {
		t.Fatal("accepted Basic for Argo CD")
	}
}
