package application

import (
	"context"
	"errors"
	"time"

	"idp-dashboard/internal/platform"
)

type revisionKey struct{}

func (a *Service) observationProvider(id string, q platform.SearchRequest) (context.Context, func(), platform.Provider, platform.Observation, error) {
	o := platform.Observation{ConnectionID: id, RequestID: randomID(), WindowStart: q.Start, WindowEnd: q.End, Completeness: "unknown"}
	if id == "" {
		return nil, nil, nil, o, errors.New("Connection is required")
	}
	ctx, done, c, err := a.begin(id, 8*time.Second)
	if err != nil {
		return nil, nil, nil, o, err
	}
	o.Revision, _ = ctx.Value(revisionKey{}).(string)
	p, err := a.provider(c)
	if err != nil {
		done()
		return nil, nil, nil, o, err
	}
	return ctx, func() { _ = platform.CloseProvider(p); done() }, p, o, nil
}
func finishObservation(scope, result platform.Observation, err error, hasData bool) platform.Observation {
	scope.ObservedAt = time.Now().UTC()
	scope.Completeness = result.Completeness
	scope.Diagnostics = result.Clone().Diagnostics
	if scope.Completeness == "" {
		scope.Completeness = "unknown"
	}
	scope.Outcome = "success"
	if scope.Completeness != "complete" {
		scope.Outcome = "partial"
	}
	if err != nil {
		scope.Outcome = "error"
		if hasData {
			scope.Outcome = "partial"
		}
		scope.Completeness = "unknown"
		scope.Diagnostics = append(scope.Diagnostics, err.Error())
	}
	return scope
}

// Upstream failures live inside the typed envelope so partial data survives Wails.
// Cancellation remains a rejected call: an obsolete revision must never publish.
func (a *Service) SearchLogs(id string, q platform.SearchRequest) (result platform.LogResult, err error) {
	ctx, done, p, scope, err := a.observationProvider(id, q)
	if err != nil {
		return result, err
	}
	defer done()
	defer func() {
		if ctx.Err() != nil {
			result = platform.LogResult{}
			err = ctx.Err()
		}
	}()
	source, ok := platform.Resolve(p, "logs").(platform.LogSource)
	if !ok {
		return result, errors.New("This provider does not support logs")
	}
	result, err = source.SearchLogs(ctx, q)
	result.Observation = finishObservation(scope, result.Observation, err, len(result.Entries) > 0)
	return result.Clone(), nil
}
func (a *Service) SearchTraces(id string, q platform.SearchRequest) (result platform.TraceSearchResult, err error) {
	ctx, done, p, scope, err := a.observationProvider(id, q)
	if err != nil {
		return result, err
	}
	defer done()
	defer func() {
		if ctx.Err() != nil {
			result = platform.TraceSearchResult{}
			err = ctx.Err()
		}
	}()
	source, ok := platform.Resolve(p, "traces").(platform.TraceSource)
	if !ok {
		return result, errors.New("This provider does not support traces")
	}
	result, err = source.SearchTraces(ctx, q)
	result.Observation = finishObservation(scope, result.Observation, err, len(result.Traces) > 0)
	return result.Clone(), nil
}
func (a *Service) Trace(id, traceID string) (result platform.TraceResult, err error) {
	ctx, done, p, scope, err := a.observationProvider(id, platform.SearchRequest{})
	if err != nil {
		return result, err
	}
	defer done()
	defer func() {
		if ctx.Err() != nil {
			result = platform.TraceResult{}
			err = ctx.Err()
		}
	}()
	source, ok := platform.Resolve(p, "traces").(platform.TraceSource)
	if !ok {
		return result, errors.New("This provider does not support traces")
	}
	result, err = source.Trace(ctx, traceID)
	result.Observation = finishObservation(scope, result.Observation, err, len(result.Spans) > 0)
	return result.Clone(), nil
}
