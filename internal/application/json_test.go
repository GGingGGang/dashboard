package application

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"idp-dashboard/internal/platform"
)

type jsonReaderFunc func(context.Context, platform.JSONRequest) (platform.JSONResult, error)

func (f jsonReaderFunc) Read(ctx context.Context, request platform.JSONRequest) (platform.JSONResult, error) {
	return f(ctx, request)
}

func TestJSONQueriesUseAdmissionAndStopOnClose(t *testing.T) {
	a, _ := newTestService(t, func(platform.Connection, string) (platform.Provider, error) { return nil, errors.New("unused") })
	started := make(chan struct{}, 3)
	finished := make(chan error, 3)
	var calls atomic.Int32
	a.jsonReader = jsonReaderFunc(func(ctx context.Context, request platform.JSONRequest) (platform.JSONResult, error) {
		calls.Add(1)
		started <- struct{}{}
		<-ctx.Done()
		return platform.JSONResult{}, ctx.Err()
	})
	for i := 0; i < 3; i++ {
		go func() {
			_, err := a.ReadJSON(platform.JSONRequest{Source: "http", URL: "http://example.invalid", Filter: "."})
			finished <- err
		}()
	}
	receive(t, started)
	receive(t, started)
	if calls.Load() != 2 {
		t.Fatal("JSON queries exceeded admission limit")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if receive(t, finished) == nil {
			t.Fatal("query survived shutdown")
		}
	}
	if _, err := a.ReadJSON(platform.JSONRequest{}); err == nil {
		t.Fatal("query accepted after shutdown")
	}
}
