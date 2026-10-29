package application

import (
	"context"
	"errors"
)

// Plugin operations may use another plugin's provider. Their cancellation is
// therefore tracked independently from the connection owner.
func (a *Service) pluginOperation(parent context.Context, id string, revision uint64) (context.Context, func(), error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	valid := false
	for _, p := range a.registry.Plugins() {
		if p.Manifest.ID == id && p.Enabled && p.Revision == revision {
			valid = true
		}
	}
	if !valid {
		return parent, nil, errors.New("Plugin changed; retry")
	}
	ctx, cancel := context.WithCancel(parent)
	key := randomID()
	if a.pluginCalls == nil {
		a.pluginCalls = map[string]map[string]context.CancelFunc{}
	}
	if a.pluginCalls[id] == nil {
		a.pluginCalls[id] = map[string]context.CancelFunc{}
	}
	a.pluginCalls[id][key] = cancel
	return ctx, func() {
		a.mu.Lock()
		delete(a.pluginCalls[id], key)
		if len(a.pluginCalls[id]) == 0 {
			delete(a.pluginCalls, id)
		}
		a.mu.Unlock()
		cancel()
	}, nil
}
func (a *Service) cancelPluginOperations(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, cancel := range a.pluginCalls[id] {
		cancel()
	}
	delete(a.pluginCalls, id)
}
