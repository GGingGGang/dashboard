package application

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/storage"
)

type logFixture struct {
	query  func(context.Context) (platform.LogResult, error)
	closed *atomic.Int32
}

func (p *logFixture) Check(context.Context) error { return nil }
func (p *logFixture) SearchLogs(ctx context.Context, _ platform.SearchRequest) (platform.LogResult, error) {
	return p.query(ctx)
}
func (p *logFixture) Close() error { p.closed.Add(1); return nil }

func TestObservationEnvelopeAndRevisionCancellation(t *testing.T) {
	var closed atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	var slow atomic.Bool
	registry, err := platform.NewRegistry(platform.Definition{Info: platform.ProviderInfo{Kind: "logs", DefaultAuth: "none", AuthMethods: []string{"none"}, Capabilities: []string{"logs"}, OnDemand: true, Queries: map[string]platform.QueryInfo{"logs": {Language: "fixture"}}}, Create: func(platform.Connection, string) (platform.Provider, error) {
		return &logFixture{closed: &closed, query: func(context.Context) (platform.LogResult, error) {
			if slow.Load() {
				close(entered)
				<-release
			}
			return platform.LogResult{Entries: []platform.LogEntry{{Line: "kept", TimestampNS: "1700000000123456789", Fields: map[string]string{"trace_id": "abc"}}}}, errors.New("partial upstream")
		}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	a := New(context.Background(), store, &memoryCredentials{values: map[string]string{}}, registry, false)
	defer a.Close()
	c := platform.Connection{Kind: "logs", Name: "logs", URL: "https://example.invalid", Auth: "none"}
	id, err := a.SaveConnection(ConnectionInput{Connection: c})
	if err != nil {
		t.Fatal(err)
	}
	c.ID = id
	r, err := a.SearchLogs(id, platform.SearchRequest{Start: 10, End: 20})
	if err != nil {
		t.Fatal(err)
	}
	if r.Observation.Outcome != "partial" || r.Observation.RequestID == "" || r.Observation.Revision != a.GetState().Connections[0].Revision || r.Observation.WindowStart != 10 || len(r.Entries) != 1 || len(r.Observation.Diagnostics) != 1 || closed.Load() != 1 {
		t.Fatalf("envelope lost: %+v", r)
	}
	slow.Store(true)
	result := make(chan error, 1)
	go func() { _, err := a.SearchLogs(id, platform.SearchRequest{}); result <- err }()
	receive(t, entered)
	c.Name = "replacement"
	if _, err := a.SaveConnection(ConnectionInput{Connection: c}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := receive(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("obsolete observation published: %v", err)
	}
	if closed.Load() != 2 {
		t.Fatal("provider not closed")
	}
}

func TestTenantCannotReuseConnectionOrCredential(t *testing.T) {
	var closed atomic.Int32
	a, c := newTestService(t, func(platform.Connection, string) (platform.Provider, error) {
		return &queryProvider{closed: &closed}, nil
	})
	c.Tenant = "other"
	if _, err := a.SaveConnection(ConnectionInput{Connection: c}); err == nil {
		t.Fatal("tenant changed on existing connection")
	}
	c.Tenant = ""
	c.Auth = "bearer"
	if _, err := a.SaveConnection(ConnectionInput{Connection: c, Secret: "token"}); err != nil {
		t.Fatal(err)
	}
	c.Tenant = "other"
	if _, err := a.TestConnection(ConnectionInput{Connection: c}); err == nil {
		t.Fatal("credential reused across tenants")
	}
}
