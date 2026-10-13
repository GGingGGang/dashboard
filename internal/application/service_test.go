package application

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/storage"
)

type memoryCredentials struct {
	mu     sync.Mutex
	values map[string]string
}

func (m *memoryCredentials) Get(ref string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.values[ref], nil
}
func (m *memoryCredentials) Set(ref, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[ref] = value
	return nil
}
func (m *memoryCredentials) Delete(ref string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, ref)
	return nil
}

// No Discover method: a query-only provider must be usable without target listing.
type queryProvider struct {
	query  func(context.Context, platform.Query) (platform.QueryResult, error)
	closed *atomic.Int32
}

func (p *queryProvider) Check(context.Context) error { return nil }
func (p *queryProvider) Query(ctx context.Context, q platform.Query) (platform.QueryResult, error) {
	return p.query(ctx, q)
}
func (p *queryProvider) Close() error { p.closed.Add(1); return nil }

type deniedDiscovery struct{ *queryProvider }

func (*deniedDiscovery) Discover(context.Context) ([]platform.Target, error) {
	return nil, &platform.APIError{Status: 403, Message: "listing denied"}
}

func newTestService(t *testing.T, create func(platform.Connection, string) (platform.Provider, error)) (*Service, platform.Connection) {
	t.Helper()
	store, err := storage.OpenStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := platform.NewRegistry(platform.Definition{
		Info:   platform.ProviderInfo{Kind: "query-only", Name: "Query only", Capabilities: []string{"metrics"}, DefaultAuth: "none", AuthMethods: []string{"none", "bearer"}, Query: &platform.QueryInfo{Language: "fixture"}, OnDemand: true},
		Create: create,
	})
	if err != nil {
		t.Fatal(err)
	}
	a := New(context.Background(), store, &memoryCredentials{values: map[string]string{}}, registry, false)
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Error(err)
		}
	})
	c := platform.Connection{Kind: "query-only", Name: "Test", URL: "https://example.invalid", Auth: "none"}
	id, err := a.SaveConnection(ConnectionInput{Connection: c})
	if err != nil {
		t.Fatal(err)
	}
	c.ID = id
	return a, c
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("operation did not finish")
		var zero T
		return zero
	}
}

func TestOnDemandAndOptionalDiscovery(t *testing.T) {
	var created, closed atomic.Int32
	a, c := newTestService(t, func(platform.Connection, string) (platform.Provider, error) {
		created.Add(1)
		return &queryProvider{closed: &closed, query: func(context.Context, platform.Query) (platform.QueryResult, error) {
			return platform.QueryResult{Type: "vector"}, nil
		}}, nil
	})
	a.Refresh()
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	if created.Load() != 0 || len(a.GetState().Snapshots) != 0 {
		t.Fatal("on-demand registration performed background work")
	}
	result, err := a.TestConnection(ConnectionInput{Connection: c})
	if err != nil || result.Discovery != "unsupported" || len(result.Targets) != 0 {
		t.Fatalf("optional discovery: %+v, %v", result, err)
	}
	if _, err := a.Query(c.ID, platform.Query{}); err != nil {
		t.Fatal(err)
	}
	if created.Load() != 2 || closed.Load() != 2 {
		t.Fatal("manual provider instances were not closed")
	}
}

func TestDiscoveryDeniedDoesNotBlockDirectQuery(t *testing.T) {
	var closed atomic.Int32
	a, c := newTestService(t, func(platform.Connection, string) (platform.Provider, error) {
		return &deniedDiscovery{&queryProvider{closed: &closed, query: func(context.Context, platform.Query) (platform.QueryResult, error) {
			return platform.QueryResult{Type: "vector"}, nil
		}}}, nil
	})
	result, err := a.TestConnection(ConnectionInput{Connection: c})
	if err != nil || result.Discovery != "failed" || result.Warning != "listing denied" {
		t.Fatalf("permission failure was lost: %+v %v", result, err)
	}
	if _, err := a.Query(c.ID, platform.Query{}); err != nil {
		t.Fatal(err)
	}
}

func TestReplacedOrDeletedConnectionRejectsLateResult(t *testing.T) {
	for _, action := range []string{"edit", "delete", "rotate"} {
		t.Run(action, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var closed atomic.Int32
			a, c := newTestService(t, func(platform.Connection, string) (platform.Provider, error) {
				return &queryProvider{closed: &closed, query: func(context.Context, platform.Query) (platform.QueryResult, error) {
					close(entered)
					<-release
					return platform.QueryResult{Type: "old-success"}, nil
				}}, nil
			})
			before := a.GetState().Connections[0].Revision
			result := make(chan error, 1)
			go func() {
				value, err := a.Query(c.ID, platform.Query{})
				if value.Type != "" {
					err = errors.New("old result escaped")
				}
				result <- err
			}()
			receive(t, entered)
			var err error
			if action == "delete" {
				err = a.DeleteConnection(c.ID)
			} else {
				input := ConnectionInput{Connection: c}
				input.Connection.Name = "Changed"
				if action == "rotate" {
					input.Connection.Auth = "bearer"
					input.Secret = "fake-secret"
				}
				_, err = a.SaveConnection(input)
			}
			close(release)
			if err != nil {
				t.Fatal(err)
			}
			if err := receive(t, result); !errors.Is(err, context.Canceled) {
				t.Fatalf("late result not rejected: %v", err)
			}
			if closed.Load() != 1 {
				t.Fatal("old provider leaked")
			}
			if action != "delete" && a.GetState().Connections[0].Revision == before {
				t.Fatal("revision was reused")
			}
		})
	}
}

func TestShutdownCancelsActiveAndQueuedWork(t *testing.T) {
	entered := make(chan struct{}, 2)
	var created, closed atomic.Int32
	a, c := newTestService(t, func(platform.Connection, string) (platform.Provider, error) {
		created.Add(1)
		return &queryProvider{closed: &closed, query: func(ctx context.Context, _ platform.Query) (platform.QueryResult, error) {
			entered <- struct{}{}
			<-ctx.Done()
			return platform.QueryResult{}, ctx.Err()
		}}, nil
	})
	finished := make(chan error, 3)
	for range 2 {
		go func() { _, err := a.Query(c.ID, platform.Query{}); finished <- err }()
	}
	receive(t, entered)
	receive(t, entered)
	go func() { _, err := a.Query(c.ID, platform.Query{}); finished <- err }()
	shutdown := make(chan error, 1)
	go func() { shutdown <- a.Close() }()
	if err := receive(t, shutdown); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := receive(t, finished); err == nil {
			t.Fatal("shutdown accepted a query")
		}
	}
	if created.Load() != 2 || closed.Load() != 2 {
		t.Fatalf("created %d, closed %d", created.Load(), closed.Load())
	}
	if _, err := a.SaveConnection(ConnectionInput{Connection: c}); err == nil {
		t.Fatal("closed service accepted changes")
	}
}

func TestAdmissionDeadlinePrecedesProviderConstruction(t *testing.T) {
	var created atomic.Int32
	a, c := newTestService(t, func(platform.Connection, string) (platform.Provider, error) {
		created.Add(1)
		return nil, errors.New("must not construct")
	})
	a.gate <- struct{}{}
	a.gate <- struct{}{}
	_, _, _, err := a.begin(c.ID, 10*time.Millisecond)
	<-a.gate
	<-a.gate
	if !errors.Is(err, context.DeadlineExceeded) || created.Load() != 0 {
		t.Fatalf("admission escaped deadline: %v", err)
	}
}
