package demo

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	sdk "github.com/GGingGGang/dashboard/sdk"
	"idp-dashboard/internal/collector"
	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/providers"
	"idp-dashboard/internal/storage"
)

func TestAllAdaptersThroughDemoAPIs(t *testing.T) {
	base, closeFn := Start()
	defer closeFn()
	s, err := storage.OpenStore(filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, c := range Connections(base) {
		p, err := providers.DefaultRegistry().New(c, "")
		if err != nil {
			t.Fatal(err)
		}
		if err = p.Check(context.Background()); err != nil {
			t.Fatal(c.Kind, err)
		}
		defer platform.CloseProvider(p)
		q := platform.SearchRequest{Expression: `{service_name="payments"}`, Start: time.Now().Unix() - 60, End: time.Now().Unix(), Limit: 10}
		if source, ok := p.(platform.LogSource); ok {
			r, err := source.SearchLogs(context.Background(), q)
			if err != nil || len(r.Entries) != 1 {
				t.Fatalf("demo logs: %+v %v", r, err)
			}
			continue
		}
		if source, ok := p.(platform.TraceSource); ok {
			q.Expression = `{ status=error }`
			r, err := source.SearchTraces(context.Background(), q)
			if err != nil || len(r.Traces) != 1 {
				t.Fatalf("demo traces: %+v %v", r, err)
			}
			detail, err := source.Trace(context.Background(), r.Traces[0].TraceID)
			if err != nil || len(detail.Spans) != 1 {
				t.Fatalf("demo span: %+v %v", detail, err)
			}
			continue
		}
		if source, ok := p.(sdk.DataSource); ok {
			result, err := source.ReadData(context.Background(), map[string]any{"filter": ".", "mode": "table"})
			if err != nil || len(result.Rows) != 1 {
				t.Fatal("demo generic data", result, err)
			}
			continue
		}
		poller := collector.NewCollector(c, p, s)
		snap := poller.Poll(context.Background())
		if snap.Error != "" || snap.StorageError != "" {
			t.Fatalf("%s: %+v", c.Kind, snap)
		}
	}
	history, err := s.History(storage.HistoryFilter{})
	if err != nil || history.Total != 320 {
		t.Fatalf("expected all 320 builds, got %d (%v)", history.Total, err)
	}
}
