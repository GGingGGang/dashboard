package demo

import (
	"context"
	"path/filepath"
	"testing"

	"idp-dashboard/internal/collector"
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
