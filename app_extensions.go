package main

import (
	"errors"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"idp-dashboard/internal/application"
	"idp-dashboard/internal/extensions"
	"idp-dashboard/internal/platform"
)

func (a *App) Plugins() []platform.PluginState { return a.registry.Plugins() }

func (a *App) SetPluginEnabled(id string, enabled bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.service.SetPluginEnabled(id, enabled)
}

func (a *App) Sources(id string) ([]platform.SourceState, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.Sources(id)
}

func (a *App) QuerySource(id, sourceID string, q platform.Query) (platform.QueryResult, error) {
	if err := a.ready(); err != nil {
		return platform.QueryResult{}, err
	}
	return a.service.QuerySource(id, sourceID, q)
}

func (a *App) ExecuteContribution(connectionID, sourceID string, request application.ContributionRequest) (sdk.DataResult, error) {
	if err := a.ready(); err != nil {
		return sdk.DataResult{}, err
	}
	return a.service.ExecuteContribution(connectionID, sourceID, request)
}

func (a *App) InstalledPlugins() ([]extensions.Installed, error) {
	if a.packages == nil {
		return nil, errors.New("Plugin directory is unavailable")
	}
	return a.packages.List()
}
func (a *App) InstallPlugin(trusted bool) (string, error) {
	if a.packages == nil {
		return "", errors.New("Plugin directory is unavailable")
	}
	if !trusted {
		return "", errors.New("Confirm that you trust this package's native code")
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Install trusted plugin ZIP", Filters: []runtime.FileFilter{{DisplayName: "Plugin ZIP", Pattern: "*.zip"}}})
	if err != nil || path == "" {
		return "", err
	}
	manifest, err := a.packages.Install(path, trusted)
	if err != nil {
		return "", err
	}
	return manifest.ID, nil
}
func (a *App) UninstallPlugin(id string) error {
	if a.packages == nil {
		return errors.New("Plugin directory is unavailable")
	}
	return a.packages.Uninstall(id)
}
