package demo

import (
	"context"
	"idp-dashboard/internal/platform"
	"path/filepath"
	"testing"
)

func TestAllAdaptersThroughDemoAPIs(t *testing.T) {
	base, closeFn := Start()
	defer closeFn()
	s, err := platform.OpenStore(filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, c := range Connections(base) {
		p, err := platform.New(c, "")
		if err != nil {
			t.Fatal(err)
		}
		if err = p.Check(context.Background()); err != nil {
			t.Fatal(c.Kind, err)
		}
		collector := platform.NewCollector(c, p, s)
		snap := collector.Poll(context.Background())
		if snap.Error != "" || snap.StorageError != "" {
			t.Fatalf("%s: %+v", c.Kind, snap)
		}
	}
	history, err := s.History(platform.HistoryFilter{})
	if err != nil || history.Total != 320 {
		t.Fatalf("expected all 320 builds, got %d (%v)", history.Total, err)
	}
}
