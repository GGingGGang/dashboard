package application

import (
	"context"
	"errors"
	"time"

	sdk "github.com/GGingGGang/dashboard/sdk"
	"idp-dashboard/internal/platform"
)

type runnerKey struct{}

// sourceScope is initialized lazily under existing per-connection admission.
// Legacy bridge operations keep their current instance lifetime until migration.
func (a *Service) sourceScope(ctx context.Context, c platform.SavedConnection) (*platform.Sources, error) {
	r, ok := ctx.Value(runnerKey{}).(*runner)
	if !ok {
		return nil, errors.New("Connection is required")
	}
	r.sourcesMu.Lock()
	defer r.sourcesMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.sources != nil {
		if r.sources.Failure() == nil {
			return r.sources, nil
		}
		r.sources.Close()
		r.sources = nil
	}
	p, err := a.provider(c)
	if err != nil {
		return nil, err
	}
	info, _ := a.registry.Info(c.Connection.Kind)
	sources := platform.NewSources(r.ctx)
	d := sdk.SourceDescriptor{ID: "default", Name: c.Connection.Name, Capabilities: platform.SourceCapabilities(info.Capabilities)}
	if err := sources.Register(ctx, d, p); err != nil {
		_ = platform.CloseProvider(p)
		sources.Close()
		return nil, err
	}
	if publisher, ok := p.(sdk.SourcePublisher); ok {
		// Hold the publisher alive even if it removes its own default source.
		_, _, _, release, err := sources.Acquire(ctx, "default")
		if err != nil {
			sources.Close()
			return nil, err
		}
		err = publisher.PublishSources(ctx, sources)
		release()
		if err != nil {
			sources.Close()
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		sources.Close()
		return nil, err
	}
	r.sources = sources
	return sources, nil
}

func (a *Service) Sources(id string) ([]platform.SourceState, error) {
	if id == "" {
		return nil, errors.New("Connection is required")
	}
	ctx, done, c, err := a.begin(id, 8*time.Second)
	if err != nil {
		return nil, err
	}
	defer done()
	sources, err := a.sourceScope(ctx, c)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return sources.List(), nil
}

// QuerySource is the first typed consumer of registered sources. Collection and
// legacy bridge calls retain their existing paths until each plugin is migrated.
func (a *Service) QuerySource(id, sourceID string, q platform.Query) (result platform.QueryResult, err error) {
	if id == "" {
		return result, errors.New("Connection is required")
	}
	ctx, done, c, err := a.begin(id, 8*time.Second)
	if err != nil {
		return result, err
	}
	defer done()
	sources, err := a.sourceScope(ctx, c)
	if err != nil {
		return result, err
	}
	callCtx, p, _, release, err := sources.Acquire(ctx, sourceID)
	if err != nil {
		return result, err
	}
	defer release()
	defer func() {
		if callCtx.Err() != nil {
			result = platform.QueryResult{}
			err = callCtx.Err()
		}
	}()
	m, ok := platform.Resolve(p, "metrics").(platform.MetricSource)
	if !ok {
		return result, errors.New("Source does not support metrics")
	}
	result, err = m.Query(callCtx, q)
	return result.Clone(), err
}
