package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func connection(kind, address string) Connection {
	return Connection{ID: "test", Kind: kind, Name: "test", URL: address, Auth: "none"}
}
func testStore(t *testing.T) *Store {
	t.Helper()
	s, e := OpenStore(filepath.Join(t.TempDir(), "archive.db"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestJenkinsFolderAndPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "reader" || p != "secret" {
			t.Error("missing authentication")
			w.WriteHeader(403)
			return
		}
		switch r.URL.EscapedPath() {
		case "/jenkins/api/json":
			fmt.Fprint(w, `{"jobs":[{"name":"team","_class":"com.cloudbees.hudson.plugins.folder.Folder"}]}`)
		case "/jenkins/job/team/api/json":
			fmt.Fprint(w, `{"jobs":[{"name":"feature%2Fone","fullName":"team/feature%2Fone","_class":"org.jenkinsci.plugins.workflow.job.WorkflowJob"}]}`)
		case "/jenkins/job/team/job/feature%252Fone/api/json":
			if !strings.Contains(r.URL.Query().Get("tree"), "{100,200}") {
				t.Error("missing page bounds")
			}
			fmt.Fprint(w, `{"builds":[{"number":4,"timestamp":1700000000000,"duration":50,"building":false,"result":"UNSTABLE","actions":[{"lastBuiltRevision":{"SHA1":"abc"}}]}]}`)
		default:
			t.Error(r.URL.String())
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := connection("jenkins", srv.URL+"/jenkins")
	c.Auth = "basic"
	c.Username = "reader"
	p, e := New(c, "secret")
	if e != nil {
		t.Fatal(e)
	}
	targets, e := p.Discover(context.Background())
	if e != nil || len(targets) != 1 {
		t.Fatalf("%v %v", targets, e)
	}
	builds, more, e := p.(CI).Builds(context.Background(), targets[0].ID, 100)
	if e != nil || more || len(builds) != 1 || builds[0].Status != "UNSTABLE" || builds[0].Commit != "abc" {
		t.Fatalf("%+v %v", builds, e)
	}
}

func TestAuthAndRedirectDoNotLeak(t *testing.T) {
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect was followed") }))
	defer destination.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 302) }))
	defer srv.Close()
	c := connection("prometheus", srv.URL)
	c.Auth = "bearer"
	p, _ := New(c, "top-secret")
	if e := p.Check(context.Background()); e == nil || strings.Contains(e.Error(), "top-secret") {
		t.Fatalf("unsafe error: %v", e)
	}
	for _, raw := range []string{"file:///etc/passwd", "https://user:pass@example.org", "https://example.org?token=secret"} {
		c.URL = raw
		if _, e := New(c, "secret"); e == nil {
			t.Error("accepted invalid URL", raw)
		}
	}
}

func TestPrometheusLimitsAndMissingValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("timeout") != "5s" || r.URL.Query().Get("limit") != "101" {
			t.Error("missing query guard")
		}
		rows := []any{}
		for i := 0; i < 102; i++ {
			rows = append(rows, map[string]any{"metric": map[string]string{"instance": fmt.Sprint(i)}, "value": []any{1700000000, "NaN"}})
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": map[string]any{"resultType": "vector", "result": rows}})
	}))
	defer srv.Close()
	p, _ := New(connection("prometheus", srv.URL), "")
	m := p.(Monitoring)
	r, e := m.Query(context.Background(), Query{Expression: "up"})
	if e != nil || !r.Truncated || len(r.Series) != 100 || r.Series[0].Points[0].Value != nil {
		t.Fatalf("%+v %v", r, e)
	}
	if _, e = m.Query(context.Background(), Query{Expression: "up", Start: 1, End: 100000}); e == nil {
		t.Error("unbounded query accepted")
	}
}

func TestTLSCustomCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"scalar","result":[1700000000,"1"]}}`)
	}))
	defer srv.Close()
	p, _ := New(connection("prometheus", srv.URL), "")
	if e := p.Check(context.Background()); e == nil {
		t.Error("untrusted certificate accepted")
	}
}

func TestArgoSeparatesSyncAndHealth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"items":[{"metadata":{"name":"app","namespace":"argocd"},"spec":{"project":"apps"},"status":{"sync":{"status":"OutOfSync","revisions":["a","b"]},"health":{"status":"Healthy"},"operationState":{"phase":"Failed","message":"sync failed"}}}]}`)
	}))
	defer srv.Close()
	p, _ := New(connection("argocd", srv.URL), "")
	apps, e := p.(CD).Deployments(context.Background())
	if e != nil || len(apps) != 1 || apps[0].Health != "Healthy" || apps[0].Sync != "OutOfSync" || len(apps[0].Revisions) != 2 || apps[0].Phase != "Failed" {
		t.Fatalf("%+v %v", apps, e)
	}
}

func TestArchiveReusedNumbersAndBackup(t *testing.T) {
	s := testStore(t)
	b := Build{ConnectionID: "one", Job: "/job/app/", Number: 1, Started: 1000, Status: "RUNNING", Observed: 2000}
	if e := s.SaveBuilds([]Build{b, b}); e != nil {
		t.Fatal(e)
	}
	b.Status = "SUCCESS"
	b.Observed = 3000
	if e := s.SaveBuilds([]Build{b}); e != nil {
		t.Fatal(e)
	}
	b.Status = "RUNNING"
	b.Observed = 4000
	_ = s.SaveBuilds([]Build{b})
	b.Started = 9000
	_ = s.SaveBuilds([]Build{b})
	b.ConnectionID = "two"
	_ = s.SaveBuilds([]Build{b})
	page, e := s.History(HistoryFilter{})
	if e != nil || page.Total != 3 {
		t.Fatalf("%+v %v", page, e)
	}
	for _, row := range page.Builds {
		if row.Started == 1000 && row.Status != "SUCCESS" {
			t.Error("terminal build regressed")
		}
	}
	backup := filepath.Join(t.TempDir(), "backup.db")
	if e = s.Backup(backup); e != nil {
		t.Fatal(e)
	}
	copy, e := OpenStore(backup)
	if e != nil {
		t.Fatal(e)
	}
	defer copy.Close()
	restored, e := copy.History(HistoryFilter{})
	if e != nil || restored.Total != 3 {
		t.Fatalf("backup %v %v", restored, e)
	}
	if e = s.Backup(backup); e == nil {
		t.Error("backup overwrote existing file")
	}
	if _, e = s.History(HistoryFilter{Job: "%' OR 1=1 --"}); e != nil {
		t.Fatal(e)
	}
}

type fakeCI struct {
	builds    []Build
	err       error
	detail    Build
	detailErr error
}

func (f *fakeCI) Check(context.Context) error                { return f.err }
func (f *fakeCI) Discover(context.Context) ([]Target, error) { return []Target{}, nil }
func (f *fakeCI) Builds(context.Context, string, int) ([]Build, bool, error) {
	return f.builds, false, f.err
}
func (f *fakeCI) Build(context.Context, string, int64) (Build, error) { return f.detail, f.detailErr }
func (f *fakeCI) Queue(context.Context) ([]QueueItem, error)          { return []QueueItem{}, f.err }
func TestCollectorSourceLossDoesNotEraseArchive(t *testing.T) {
	s := testStore(t)
	c := connection("jenkins", "http://localhost")
	c.Targets = []Target{{ID: "/job/app/"}}
	b := Build{ConnectionID: c.ID, Job: c.Targets[0].ID, Number: 1, Started: 1000, Status: "RUNNING", Observed: 2000}
	f := &fakeCI{builds: []Build{b}, detail: b}
	collector := NewCollector(c, f, s)
	first := collector.Poll(context.Background())
	if first.Error != "" {
		t.Fatal(first.Error)
	}
	f.builds = []Build{}
	f.detailErr = &APIError{Status: 404, Message: "not found"}
	collector.Poll(context.Background())
	p, e := s.History(HistoryFilter{})
	if e != nil || p.Total != 1 || p.Builds[0].Status != "UNCONFIRMED" {
		t.Fatalf("%+v %v", p, e)
	}
	f.err = fmt.Errorf("offline")
	snap := collector.Poll(context.Background())
	if snap.Error == "" || snap.LastSuccess.IsZero() {
		t.Fatal("lost freshness evidence")
	}
}

func TestPrometheusLive(t *testing.T) {
	address := os.Getenv("IDP_TEST_PROMETHEUS_URL")
	if address == "" {
		t.Skip("set IDP_TEST_PROMETHEUS_URL for read-only integration test")
	}
	p, e := New(connection("prometheus", address), "")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	end := time.Now().Unix()
	r, e := p.(Monitoring).Query(ctx, Query{Expression: "up", Start: end - 3600, End: end})
	if e != nil || len(r.Series) == 0 {
		t.Fatalf("range query failed: %v", e)
	}
	t.Logf("read %d series; %d samples in first series", len(r.Series), len(r.Series[0].Points))
}

func TestFutureDatabaseVersionIsNotOverwritten(t *testing.T) {
	s := testStore(t)
	if _, err := s.db.Exec("PRAGMA user_version=99"); err != nil {
		t.Fatal(err)
	}
	if other, err := OpenStore(s.Path); err == nil {
		other.Close()
		t.Fatal("opened a newer schema")
	}
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 99 {
		t.Fatal("changed future schema version")
	}
}
