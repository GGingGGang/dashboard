package main

import (
	"context"
	"errors"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"github.com/GGingGGang/dashboard/sdk/protocol"
	"os"
	"time"
)

type provider struct {
	host   sdk.SourceRegistrar
	value  string
	secret string
}

func (p *provider) Check(context.Context) error { return nil }
func (p *provider) PublishSources(ctx context.Context, host sdk.SourceRegistrar) error {
	p.host = host
	return host.Register(ctx, sdk.SourceDescriptor{ID: "extra", Name: "Extra", Capabilities: []string{"data"}}, &provider{value: "old"})
}
func (p *provider) ReadData(ctx context.Context, input map[string]any) (sdk.DataResult, error) {
	switch input["action"] {
	case "update":
		return sdk.DataResult{Type: "scalar", Value: "updated"}, p.host.Update(ctx, sdk.SourceDescriptor{ID: "extra", Name: "Extra", Capabilities: []string{"data"}}, &provider{value: "new"})
	case "delete":
		return sdk.DataResult{Type: "scalar", Value: "deleted"}, p.host.Delete(ctx, "extra")
	case "crash":
		os.Exit(9)
	case "slow":
		time.Sleep(30 * time.Second)
	case "secret":
		return sdk.DataResult{}, errors.New("cannot authenticate: " + p.secret)
	}
	return sdk.DataResult{Type: "scalar", Value: p.value}, nil
}
func main() {
	_ = protocol.Serve(os.Stdin, os.Stdout, sdk.Definition{Info: sdk.ProviderInfo{Kind: "external", Name: "External", Capabilities: []string{"data"}, OnDemand: true, DefaultAuth: "none", AuthMethods: []string{"none"}}, Create: func(c sdk.Connection, secret string) (sdk.Provider, error) {
		return &provider{value: "root", secret: secret}, nil
	}})
}
