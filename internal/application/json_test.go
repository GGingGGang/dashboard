package application

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	sdk "github.com/GGingGGang/dashboard/sdk"
	"idp-dashboard/internal/platform"
)

type jsonReaderFunc func(context.Context, platform.JSONRequest) (platform.JSONResult, error)

func (f jsonReaderFunc) Check(context.Context) error { return nil }
func (f jsonReaderFunc) ReadJSON(ctx context.Context, request platform.JSONRequest) (platform.JSONResult, error) {
	return f(ctx, request)
}

func TestJSONQueriesUseAdmissionAndStopOnClose(t *testing.T) {
	a, _ := newTestService(t, func(platform.Connection, string) (platform.Provider, error) { return nil, errors.New("unused") })
	started := make(chan struct{}, 3)
	finished := make(chan error, 3)
	var calls atomic.Int32
	reader := jsonReaderFunc(func(ctx context.Context, request platform.JSONRequest) (platform.JSONResult, error) {
		calls.Add(1)
		started <- struct{}{}
		<-ctx.Done()
		return platform.JSONResult{}, ctx.Err()
	})

	if err := a.RegisterPlugin(sdk.Manifest{ID: "json", Version: "1", ContractVersion: 1, Providers: []sdk.ProviderContribution{{ID: "json"}}}, sdk.Definition{Info: sdk.ProviderInfo{Kind: "json", Name: "JSON", Capabilities: []string{"json"}, OnDemand: true, DefaultAuth: "none", AuthMethods: []string{"none"}}, Create: func(sdk.Connection, string) (sdk.Provider, error) { return &reader, nil }}); err != nil {
		t.Fatal(err)
	}
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

func TestDisableJSONCancelsLegacyQueries(t *testing.T) {
	a, _ := newTestService(t, func(platform.Connection, string) (platform.Provider, error) { return nil, errors.New("unused") })
	started := make(chan struct{})
	reader := jsonReaderFunc(func(ctx context.Context, _ platform.JSONRequest) (platform.JSONResult, error) {
		close(started)
		<-ctx.Done()
		return platform.JSONResult{Text: "obsolete"}, nil
	})
	if err := a.RegisterPlugin(sdk.Manifest{ID: "json", Version: "1", ContractVersion: 1, Providers: []sdk.ProviderContribution{{ID: "json"}}}, sdk.Definition{Info: sdk.ProviderInfo{Kind: "json", Name: "JSON", Capabilities: []string{"json"}, OnDemand: true, DefaultAuth: "none", AuthMethods: []string{"none"}}, Create: func(sdk.Connection, string) (sdk.Provider, error) { return &reader, nil }}); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() {
		result, err := a.ReadJSON(platform.JSONRequest{Source: "http", URL: "http://example.invalid", Filter: "."})
		if result.Text != "" {
			err = errors.New("obsolete result escaped")
		}
		finished <- err
	}()
	receive(t, started)
	if err := a.SetPluginEnabled("json", false); err != nil {
		t.Fatal(err)
	}
	if err := receive(t, finished); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
