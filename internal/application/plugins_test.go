package application

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	sdk "github.com/GGingGGang/dashboard/sdk"
	"idp-dashboard/internal/platform"
)

func TestDisableCancelsQueriesAndPreservesConnection(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var closed atomic.Int32
	a, c := newTestService(t, func(platform.Connection, string) (platform.Provider, error) {
		return &queryProvider{closed: &closed, query: func(context.Context, platform.Query) (platform.QueryResult, error) {
			close(entered)
			<-release
			return platform.QueryResult{Type: "stale"}, nil
		}}, nil
	})
	finished := make(chan error, 1)
	go func() {
		result, err := a.Query(c.ID, platform.Query{})
		if result.Type != "" {
			err = errors.New("disabled result escaped")
		}
		finished <- err
	}()
	receive(t, entered)
	if err := a.SetPluginEnabled("query-only", false); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := receive(t, finished); !errors.Is(err, context.Canceled) {
		t.Fatalf("late response: %v", err)
	}
	if len(a.GetState().Connections) != 1 || a.registry.Enabled(c.Kind) {
		t.Fatal("connection deleted or plugin still enabled")
	}
	if _, err := a.Query(c.ID, platform.Query{}); err == nil {
		t.Fatal("disabled plugin queried")
	}
	if err := a.Start(); err != nil {
		t.Fatal(err)
	}
	if a.registry.Enabled(c.Kind) {
		t.Fatal("disabled setting not restored")
	}
	if err := a.SetPluginEnabled("query-only", true); err != nil {
		t.Fatal(err)
	}
	if !a.registry.Enabled(c.Kind) || a.GetState().Connections[0].Revision == "" {
		t.Fatal("reactivation failed")
	}
}

type publicationProvider struct {
	*queryProvider
	publish func(context.Context, sdk.SourceRegistrar) error
}

func (p *publicationProvider) PublishSources(ctx context.Context, host sdk.SourceRegistrar) error {
	return p.publish(ctx, host)
}

func TestPublishedSourceUpdateAndDeletionThroughScopedHost(t *testing.T) {
	var rootClosed, oldClosed, nextClosed atomic.Int32
	var host sdk.SourceRegistrar
	entered, release := make(chan struct{}), make(chan struct{})
	a, c := newTestService(t, func(platform.Connection, string) (platform.Provider, error) {
		return &publicationProvider{queryProvider: &queryProvider{closed: &rootClosed}, publish: func(ctx context.Context, registrar sdk.SourceRegistrar) error {
			host = registrar
			return registrar.Register(ctx, sdk.SourceDescriptor{ID: "cpu", Name: "CPU", Capabilities: []string{"metrics"}}, &queryProvider{closed: &oldClosed, query: func(context.Context, platform.Query) (platform.QueryResult, error) {
				close(entered)
				<-release
				return platform.QueryResult{Type: "obsolete"}, nil
			}})
		}}, nil
	})
	sources, err := a.Sources(c.ID)
	if err != nil || len(sources) != 2 {
		t.Fatalf("publication: %+v %v", sources, err)
	}
	finished := make(chan error, 1)
	go func() {
		result, err := a.QuerySource(c.ID, "cpu", platform.Query{})
		if result.Type != "" {
			err = errors.New("obsolete source result escaped")
		}
		finished <- err
	}()
	receive(t, entered)
	d := sdk.SourceDescriptor{ID: "cpu", Name: "CPU", Capabilities: []string{"metrics"}}
	next := &queryProvider{closed: &nextClosed, query: func(context.Context, platform.Query) (platform.QueryResult, error) {
		return platform.QueryResult{Type: "replacement"}, nil
	}}
	if err := host.Update(context.Background(), d, next); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := receive(t, finished); !errors.Is(err, context.Canceled) {
		t.Fatalf("source cancellation: %v", err)
	}
	if oldClosed.Load() != 1 {
		t.Fatal("old source leaked")
	}
	result, err := a.QuerySource(c.ID, "cpu", platform.Query{})
	if err != nil || result.Type != "replacement" {
		t.Fatalf("replacement: %+v %v", result, err)
	}
	if err := host.Delete(context.Background(), "cpu"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.QuerySource(c.ID, "cpu", platform.Query{}); err == nil {
		t.Fatal("deleted source queried")
	}
	if nextClosed.Load() != 1 {
		t.Fatal("deleted implementation leaked")
	}
	if err := a.SetPluginEnabled("query-only", false); err != nil {
		t.Fatal(err)
	}
	if rootClosed.Load() != 1 {
		t.Fatal("publisher leaked")
	}
	if err := host.Register(context.Background(), d, next); err == nil {
		t.Fatal("disabled publisher retained access")
	}
}

type failPreferences struct{ Store }

func (s failPreferences) SavePreference(string, string) error { return errors.New("disk full") }

func TestFailedPluginSettingWriteDoesNotStopRunner(t *testing.T) {
	a, c := newTestService(t, func(platform.Connection, string) (platform.Provider, error) { return nil, errors.New("unused") })
	a.store = failPreferences{a.store}
	before := a.GetState().Connections[0].Revision
	if err := a.SetPluginEnabled("query-only", false); err == nil {
		t.Fatal("ignored setting write failure")
	}
	if !a.registry.Enabled(c.Kind) || a.GetState().Connections[0].Revision != before {
		t.Fatal("failed update damaged existing runner")
	}
}

func TestFailedPublicationRollsBackAndCanRetry(t *testing.T) {
	var closed atomic.Int32
	var attempts atomic.Int32
	a, c := newTestService(t, func(platform.Connection, string) (platform.Provider, error) {
		return &publicationProvider{queryProvider: &queryProvider{closed: &closed}, publish: func(ctx context.Context, host sdk.SourceRegistrar) error {
			if err := host.Register(ctx, sdk.SourceDescriptor{ID: "extra", Name: "Extra", Capabilities: []string{"metrics"}}, &queryProvider{closed: &closed}); err != nil {
				return err
			}
			if attempts.Add(1) == 1 {
				return errors.New("publication failed")
			}
			return nil
		}}, nil
	})
	if _, err := a.Sources(c.ID); err == nil {
		t.Fatal("ignored publication failure")
	}
	if closed.Load() != 2 {
		t.Fatal("partial publication leaked resources")
	}
	if sources, err := a.Sources(c.ID); err != nil || len(sources) != 2 {
		t.Fatalf("retry: %+v %v", sources, err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if closed.Load() != 4 {
		t.Fatal("successful publication leaked resources")
	}
}
