package loki

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"idp-dashboard/internal/platform"
)

func TestSearchPreservesPrecisionDuplicatesAndTenant(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/loki/api/v1/query_range" || r.URL.Query().Get("start") != "1700000000000000000" || r.URL.Query().Get("direction") != "backward" || r.Header.Get("X-Scope-OrgID") != "team-a" || r.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"status":"success","warnings":["fixture-secret warning"],"data":{"resultType":"streams","result":[{"stream":{"service_name":"api"},"values":[["1700000000123456789","same",{"trace_id":"ab"}],["1700000000123456789","same"]]},{"stream":{"service_name":"worker"},"values":[["1700000000123456790","newer"]]}]}}`)
	}))
	defer s.Close()
	p, err := Definition().Create(platform.Connection{Name: "Loki", URL: s.URL, Auth: "bearer", Tenant: "team-a"}, "fixture-secret")
	if err != nil {
		t.Fatal(err)
	}
	defer platform.CloseProvider(p)
	q := platform.SearchRequest{Expression: `{service_name="api"}`, Start: 1700000000, End: 1700000001, Limit: 3}
	r, err := p.(platform.LogSource).SearchLogs(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Entries) != 3 || r.Entries[0].TimestampNS != "1700000000123456790" || r.Entries[1].TimestampNS != r.Entries[2].TimestampNS || r.Entries[1].Fields["trace_id"] != "ab" || r.Observation.Completeness != "unknown" {
		t.Fatalf("lost log semantics: %+v", r)
	}
	if strings.Contains(strings.Join(r.Observation.Diagnostics, " "), "fixture-secret") {
		t.Fatal("secret in diagnostics")
	}
	r.Entries[1].Labels["service_name"] = "changed"
	if r.Entries[2].Labels["service_name"] != "api" {
		t.Fatal("log entries share mutable maps")
	}
	q.Limit = 501
	if _, err = p.(platform.LogSource).SearchLogs(context.Background(), q); err == nil || calls != 1 {
		t.Fatal("invalid query reached network")
	}
}

func TestSearchRejectsMetricAndMalformedResponses(t *testing.T) {
	for _, body := range []string{`{"status":"success","data":{"resultType":"matrix"}}`, `{"status":"success","data":{"resultType":"streams","result":[{"values":[[1700000000123456789,"bad"]]}]}}`} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		p, err := Definition().Create(platform.Connection{Name: "Loki", URL: s.URL, Auth: "none"}, "")
		if err != nil {
			t.Fatal(err)
		}
		_, err = p.(platform.LogSource).SearchLogs(context.Background(), platform.SearchRequest{Expression: `{job="api"}`, Start: 1700000000, End: 1700000001, Limit: 10})
		_ = platform.CloseProvider(p)
		s.Close()
		if err == nil {
			t.Fatal("invalid response accepted")
		}
	}
}
