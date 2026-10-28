package main

import "idp-dashboard/internal/platform"

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
