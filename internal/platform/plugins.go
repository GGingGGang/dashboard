package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"

	sdk "github.com/GGingGGang/dashboard/sdk"
)

type PluginState struct {
	Manifest sdk.Manifest `json:"manifest"`
	Enabled  bool         `json:"enabled"`
}

var extensionID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

func cloneManifest(m sdk.Manifest) sdk.Manifest {
	// Only validated JSON values enter the registry.
	data, _ := json.Marshal(m)
	var clone sdk.Manifest
	_ = json.Unmarshal(data, &clone)
	return clone
}

// RegisterPlugin validates everything before publishing any definition. Factories
// are never executed here. Runtime mutations are coordinated by application.
func (r *Registry) RegisterPlugin(m sdk.Manifest, definitions ...Definition) error {
	if !extensionID.MatchString(m.ID) || m.Version == "" || len(m.Version) > 128 {
		return errors.New("Plugin requires a valid ID and version")
	}
	if m.ContractVersion != sdk.ContractVersion {
		return errors.New("Unsupported plugin contract version")
	}
	if _, err := json.Marshal(m); err != nil {
		return errors.New("Invalid plugin schema JSON")
	}
	normalized, err := normalizeDefinitions(definitions)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, p := range m.Providers {
		if !extensionID.MatchString(p.ID) || seen[p.ID] {
			return errors.New("Invalid or duplicate provider contribution")
		}
		seen[p.ID] = true
		if _, ok := normalized[p.ID]; !ok {
			return fmt.Errorf("Missing provider implementation: %s", p.ID)
		}
	}
	if len(seen) != len(normalized) {
		return errors.New("Undeclared provider implementation")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.plugins[m.ID]; ok {
		return errors.New("Plugin is already registered")
	}
	for kind := range normalized {
		if _, ok := r.definitions[kind]; ok {
			return fmt.Errorf("Duplicate provider: %s", kind)
		}
	}
	for kind, d := range normalized {
		r.definitions[kind] = d
		r.owners[kind] = m.ID
	}
	r.plugins[m.ID] = PluginState{Manifest: cloneManifest(m), Enabled: true}
	return nil
}

func (r *Registry) Plugins() []PluginState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]PluginState, 0, len(r.plugins))
	for _, p := range r.plugins {
		p.Manifest = cloneManifest(p.Manifest)
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.ID < out[j].Manifest.ID })
	return out
}

func (r *Registry) Owner(kind string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.owners[kind]
}

func (r *Registry) Enabled(kind string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.plugins[r.owners[kind]].Enabled
}

// SetPluginEnabled must be called by the application lifecycle coordinator,
// which also persists the choice and cancels all affected connection runners.
func (r *Registry) SetPluginEnabled(id string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.plugins[id]
	if !ok {
		return errors.New("Plugin not found")
	}
	p.Enabled = enabled
	r.plugins[id] = p
	return nil
}
