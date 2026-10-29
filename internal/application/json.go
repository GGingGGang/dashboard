package application

import (
	"errors"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"idp-dashboard/internal/platform"
	"net/url"
	"time"
)

// ReadJSON preserves the existing ad-hoc API/card format while dispatching via
// the registered JSON plugin. Its transient scope never saves a connection.
func (a *Service) ReadJSON(request platform.JSONRequest) (result platform.JSONResult, err error) {
	ctx, done, _, err := a.begin("", 25*time.Second)
	if err != nil {
		return result, err
	}
	defer done()
	owner := a.registry.Owner("json")
	var revision uint64
	for _, p := range a.registry.Plugins() {
		if p.Manifest.ID == owner && p.Enabled {
			revision = p.Revision
		}
	}
	if revision == 0 {
		return result, errors.New("HTTP JSON plugin is disabled or unavailable")
	}
	ctx, releasePlugin, err := a.pluginOperation(ctx, owner, revision)
	if err != nil {
		return result, err
	}
	defer releasePlugin()
	u, err := url.Parse(request.URL)
	if err != nil || u.User != nil || request.Source != "http" {
		return result, errors.New("Use an HTTP JSON URL without credentials")
	}
	u.RawQuery = ""
	u.Fragment = ""
	provider, err := a.registry.New(sdk.Connection{Kind: "json", Name: "HTTP JSON query", URL: u.String(), Auth: "none"}, "")
	if err != nil {
		return result, err
	}
	scope := platform.NewSources(ctx)
	defer scope.Close()
	info, _ := a.registry.Info("json")
	if err = scope.Register(ctx, sdk.SourceDescriptor{ID: "default", Name: "HTTP JSON", Capabilities: platform.SourceCapabilities(info.Capabilities)}, provider); err != nil {
		_ = sdk.CloseProvider(provider)
		return result, err
	}
	callCtx, p, _, release, err := scope.Acquire(ctx, "default")
	if err != nil {
		return result, err
	}
	defer release()
	defer func() {
		if callCtx.Err() != nil {
			result = platform.JSONResult{}
			err = callCtx.Err()
		}
	}()
	reader, ok := sdk.Resolve(p, "json").(sdk.JSONSource)
	if !ok {
		return result, errors.New("Provider does not implement JSON queries")
	}
	return reader.ReadJSON(callCtx, request)
}
