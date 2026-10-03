package collector

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/storage"
)

func connection(kind, address string) platform.Connection {
	return platform.Connection{ID: "test", Kind: kind, Name: "test", URL: address, Auth: "none"}
}
func testStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.OpenStore(filepath.Join(t.TempDir(), "archive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

type fakeCI struct {
	builds    []platform.Build
	err       error
	detail    platform.Build
	detailErr error
}

func (f *fakeCI) Check(context.Context) error { return f.err }
func (f *fakeCI) Discover(context.Context) ([]platform.Target, error) {
	return []platform.Target{}, nil
}
func (f *fakeCI) Builds(context.Context, string, int) ([]platform.Build, bool, error) {
	return f.builds, false, f.err
}
func (f *fakeCI) Build(context.Context, string, int64) (platform.Build, error) {
	return f.detail, f.detailErr
}
func (f *fakeCI) Queue(context.Context) ([]platform.QueueItem, error) {
	return []platform.QueueItem{}, f.err
}

func TestCollectorSourceLossDoesNotEraseArchive(t *testing.T) {
	s := testStore(t)
	c := connection("jenkins", "http://localhost")
	c.Targets = []platform.Target{{ID: "/job/app/"}}
	b := platform.Build{ConnectionID: c.ID, Job: c.Targets[0].ID, Number: 1, Started: 1000, Status: "RUNNING", Observed: 2000}
	f := &fakeCI{builds: []platform.Build{b}, detail: b}
	collector := NewCollector(c, f, s)
	first := collector.Poll(context.Background())
	if first.Error != "" {
		t.Fatal(first.Error)
	}
	f.builds = []platform.Build{}
	f.detailErr = &platform.APIError{Status: 404, Message: "not found"}
	collector.Poll(context.Background())
	p, e := s.History(storage.HistoryFilter{})
	if e != nil || p.Total != 1 || p.Builds[0].Status != "UNCONFIRMED" {
		t.Fatalf("%+v %v", p, e)
	}
	f.err = fmt.Errorf("offline")
	snap := collector.Poll(context.Background())
	if snap.Error == "" || snap.LastSuccess.IsZero() {
		t.Fatal("lost freshness evidence")
	}
}
