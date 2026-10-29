package application

import (
	"encoding/json"
	"errors"
	"sort"

	sdk "github.com/GGingGGang/dashboard/sdk"
	"idp-dashboard/internal/platform"
)

const disabledPluginsKey = "plugins.disabled"

func (a *Service) disabledPlugins() (map[string]bool, error) {
	raw, err := a.store.Preference(disabledPluginsKey)
	if err != nil {
		return nil, errors.New("Cannot read plugin settings")
	}
	ids := []string{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &ids); err != nil {
			return nil, errors.New("Invalid plugin settings; original value preserved")
		}
	}
	result := map[string]bool{}
	for _, id := range ids {
		result[id] = true
	}
	return result, nil
}

func (a *Service) restorePlugins() error {
	disabled, err := a.disabledPlugins()
	if err != nil {
		return err
	}
	for _, p := range a.registry.Plugins() {
		if err := a.registry.SetPluginEnabled(p.Manifest.ID, !disabled[p.Manifest.ID]); err != nil {
			return err
		}
	}
	return nil
}

func (a *Service) Plugins() []platform.PluginState { return a.registry.Plugins() }

// RegisterPlugin is an in-process composition API, not an arbitrary frontend
// factory injection endpoint. External executable loading comes later.
func (a *Service) RegisterPlugin(m sdk.Manifest, definitions ...platform.Definition) error {
	a.editMu.Lock()
	defer a.editMu.Unlock()
	if err := a.ready(); err != nil {
		return err
	}
	disabled, err := a.disabledPlugins()
	if err != nil {
		return err
	}
	connections, err := a.store.Connections()
	if err != nil {
		return errors.New("Cannot read connection settings")
	}
	if err := a.registry.RegisterPlugin(m, definitions...); err != nil {
		return err
	}
	if err := a.registry.SetPluginEnabled(m.ID, !disabled[m.ID]); err != nil {
		return err
	}
	for _, c := range connections {
		if a.registry.Owner(c.Connection.Kind) == m.ID {
			a.start(c)
		}
	}
	return nil
}

func (a *Service) SetPluginEnabled(id string, enabled bool) error {
	a.editMu.Lock()
	defer a.editMu.Unlock()
	if err := a.ready(); err != nil {
		return err
	}
	found := false
	for _, p := range a.registry.Plugins() {
		if p.Manifest.ID == id {
			if p.Enabled == enabled {
				return nil
			}
			found = true
		}
	}
	if !found {
		return errors.New("Plugin not found")
	}
	disabled, err := a.disabledPlugins()
	if err != nil {
		return err
	}
	connections, err := a.store.Connections()
	if err != nil {
		return errors.New("Cannot read connection settings")
	}
	if enabled {
		delete(disabled, id)
	} else {
		disabled[id] = true
	}
	ids := make([]string, 0, len(disabled))
	for id := range disabled {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	data, _ := json.Marshal(ids)
	// Persist first: a failed write must not interrupt working instances.
	if err := a.store.SavePreference(disabledPluginsKey, string(data)); err != nil {
		return errors.New("Cannot save plugin settings")
	}
	if err := a.registry.SetPluginEnabled(id, enabled); err != nil {
		return err
	}
	a.cancelPluginOperations(id)
	for _, c := range connections {
		if a.registry.Owner(c.Connection.Kind) != id {
			continue
		}
		a.stop(c.Connection.ID)
		a.mu.Lock()
		delete(a.snapshots, c.Connection.ID)
		a.mu.Unlock()
		a.start(c)
	}
	return nil
}
