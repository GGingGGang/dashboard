package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/GGingGGang/dashboard/sdk"
	"github.com/GGingGGang/dashboard/sdk/httpapi"
)

func TestIndependentClientPreservesTransportBoundaries(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("followed redirect") }))
	defer destination.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			t.Error("missing configured authentication")
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, destination.URL, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":42}`))
	}))
	defer srv.Close()
	c := sdk.Connection{Name: "fixture", URL: srv.URL, Auth: "bearer"}
	client, err := httpapi.New(c, "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	var out struct {
		Value int `json:"value"`
	}
	if err := client.Get(context.Background(), "/value", nil, &out); err != nil || out.Value != 42 {
		t.Fatalf("read: %+v, %v", out, err)
	}
	if err := client.Get(context.Background(), "/redirect", nil, &out); err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("unsafe redirect: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.Get(ctx, "/value", nil, &out); err == nil {
		t.Fatal("ignored cancellation")
	}
}
