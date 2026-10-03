package jenkins

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"idp-dashboard/internal/platform"
)

func connection(kind, address string) platform.Connection {
	return platform.Connection{ID: "test", Kind: kind, Name: "test", URL: address, Auth: "none"}
}
func testProvider(c platform.Connection, secret string) (platform.Provider, error) {
	r, err := platform.NewRegistry(Definition())
	if err != nil {
		return nil, err
	}
	return r.New(c, secret)
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
	p, e := testProvider(c, "secret")
	if e != nil {
		t.Fatal(e)
	}
	targets, e := p.Discover(context.Background())
	if e != nil || len(targets) != 1 {
		t.Fatalf("%v %v", targets, e)
	}
	builds, more, e := p.(platform.CI).Builds(context.Background(), targets[0].ID, 100)
	if e != nil || more || len(builds) != 1 || builds[0].Status != "UNSTABLE" || builds[0].Commit != "abc" {
		t.Fatalf("%+v %v", builds, e)
	}
}
