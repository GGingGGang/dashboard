package storage

import (
	"path/filepath"
	"testing"

	"idp-dashboard/internal/platform"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "archive.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestArchiveReusedNumbersAndBackup(t *testing.T) {
	s := testStore(t)
	b := platform.Build{ConnectionID: "one", Job: "/job/app/", Number: 1, Started: 1000, Status: "RUNNING", Observed: 2000}
	if e := s.SaveBuilds([]platform.Build{b, b}); e != nil {
		t.Fatal(e)
	}
	b.Status = "SUCCESS"
	b.Observed = 3000
	if e := s.SaveBuilds([]platform.Build{b}); e != nil {
		t.Fatal(e)
	}
	b.Status = "RUNNING"
	b.Observed = 4000
	_ = s.SaveBuilds([]platform.Build{b})
	b.Started = 9000
	_ = s.SaveBuilds([]platform.Build{b})
	b.ConnectionID = "two"
	_ = s.SaveBuilds([]platform.Build{b})
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
