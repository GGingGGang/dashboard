package application

import (
	"context"
	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/providers"
	"idp-dashboard/internal/storage"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestJenkinsCollectsPublishedSourceAndPreservesArchive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/queue/api/json" {
			_, _ = w.Write([]byte(`{"items":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"builds":[{"number":1,"timestamp":1000,"duration":500,"result":"SUCCESS","building":false}]}`))
	}))
	defer server.Close()
	store, err := storage.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	a := New(context.Background(), store, &memoryCredentials{values: map[string]string{}}, providers.DefaultRegistry(), false)
	defer a.Close()
	id, err := a.SaveConnection(ConnectionInput{Connection: platform.Connection{Kind: "jenkins", Name: "Jenkins", URL: server.URL, Auth: "none", Targets: []platform.Target{{ID: "/job/app/", Name: "app"}}}})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		page, err := store.History(storage.HistoryFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no collected builds")
		}
		time.Sleep(10 * time.Millisecond)
	}
	sources, err := a.Sources(id)
	if err != nil || len(sources) != 1 || sources[0].Revision < 2 {
		t.Fatalf("publisher not used: %+v %v", sources, err)
	}
	for _, enabled := range []bool{false, true} {
		if err := a.SetPluginEnabled("jenkins", enabled); err != nil {
			t.Fatal(err)
		}
	}
	if a.GetState().Connections[0].ID != id {
		t.Fatal("connection identity changed")
	}
	page, err := store.History(storage.HistoryFilter{})
	if err != nil || page.Total != 1 {
		t.Fatal("archive lost", err)
	}
}
