package prometheus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

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
	p, _ := testProvider(connection("prometheus", srv.URL), "")
	m := p.(platform.Monitoring)
	r, e := m.Query(context.Background(), platform.Query{Expression: "up"})
	if e != nil || !r.Truncated || len(r.Series) != 100 || r.Series[0].Points[0].Value != nil {
		t.Fatalf("%+v %v", r, e)
	}
	if _, e = m.Query(context.Background(), platform.Query{Expression: "up", Start: 1, End: 100000}); e == nil {
		t.Error("unbounded query accepted")
	}
}
func TestPrometheusLive(t *testing.T) {
	address := os.Getenv("IDP_TEST_PROMETHEUS_URL")
	if address == "" {
		t.Skip("set IDP_TEST_PROMETHEUS_URL for read-only integration test")
	}
	p, e := testProvider(connection("prometheus", address), "")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	end := time.Now().Unix()
	r, e := p.(platform.Monitoring).Query(ctx, platform.Query{Expression: "up", Start: end - 3600, End: end})
	if e != nil || len(r.Series) == 0 {
		t.Fatalf("range query failed: %v", e)
	}
	t.Logf("read %d series; %d samples in first series", len(r.Series), len(r.Series[0].Points))
}
