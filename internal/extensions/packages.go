package extensions

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"idp-dashboard/internal/platform"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const maxPackage = 128 * 1024 * 1024

var safeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
var digestID = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Installed struct {
	Manifest       sdk.Manifest `json:"manifest"`
	PendingRestart bool         `json:"pendingRestart"`
	PendingRemoval bool         `json:"pendingRemoval"`
	Error          string       `json:"error,omitempty"`
}
type Manager struct {
	root     string
	mu       sync.Mutex
	loaded   map[string]string
	running  map[string]sdk.Manifest
	problems map[string]string
	builtins map[string]bool
}

func NewManager(root string, builtins []platform.PluginState) *Manager {
	m := &Manager{root: root, loaded: map[string]string{}, running: map[string]sdk.Manifest{}, problems: map[string]string{}, builtins: map[string]bool{}}
	for _, p := range builtins {
		m.builtins[p.Manifest.ID] = true
		for _, provider := range p.Manifest.Providers {
			m.builtins[provider.ID] = true
		}
	}
	return m
}
func directory(path string) string { return filepath.Dir(path) }
func (m *Manager) index() (map[string]string, error) {
	raw, err := os.ReadFile(filepath.Join(m.root, "index.json"))
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > 1024*1024 {
		return nil, errors.New("Plugin index exceeds limit")
	}
	var index map[string]string
	if err = json.Unmarshal(raw, &index); err != nil || index == nil {
		return nil, errors.New("Invalid plugin index; original preserved")
	}
	for id, digest := range index {
		if !safeID.MatchString(id) || !digestID.MatchString(digest) {
			return nil, errors.New("Invalid plugin index entry")
		}
	}
	return index, nil
}
func (m *Manager) save(index map[string]string) error {
	raw, err := json.Marshal(index)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(m.root, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(m.root, "index-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(raw); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(m.root, "index.json"))
}
func validPath(name string) bool {
	if name == "" || strings.ContainsAny(name, `\:`) || strings.HasPrefix(name, "/") {
		return false
	}
	for _, part := range strings.Split(strings.TrimSuffix(name, "/"), "/") {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, " .") != part || strings.ContainsAny(part, "<>\"|?*\x00") {
			return false
		}
		for _, r := range part {
			if r < 32 {
				return false
			}
		}
		base := strings.ToUpper(strings.Split(part, ".")[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || regexp.MustCompile(`^(COM|LPT)[0-9]$`).MatchString(base) {
			return false
		}
	}
	return true
}
func readManifest(path string) (sdk.Manifest, error) {
	var m sdk.Manifest
	f, err := os.Open(path)
	if err != nil {
		return m, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 256*1024+1))
	if err != nil || len(raw) > 256*1024 {
		return m, errors.New("Manifest exceeds limit")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&m); err != nil {
		return m, err
	}
	if !json.Valid(raw) {
		return m, errors.New("Invalid manifest")
	}
	return m, nil
}
func definitions(root string, m sdk.Manifest) ([]sdk.Definition, error) {
	definitions := []sdk.Definition{}
	for _, p := range m.Providers {
		if p.Info == nil || p.Info.Kind != p.ID || p.Executable == "" || !validPath(p.Executable) || !strings.EqualFold(filepath.Ext(p.Executable), ".exe") {
			return nil, errors.New("External providers require matching info and an in-package .exe")
		}
		path := filepath.Join(root, filepath.FromSlash(p.Executable))
		file, err := os.Lstat(path)
		if err != nil || !file.Mode().IsRegular() {
			return nil, errors.New("Provider executable is missing or not a regular file")
		}
		info := p.Info.Clone()
		seen := map[string]bool{}
		actual := 0
		for _, capability := range info.Capabilities {
			if seen[capability] {
				return nil, errors.New("Duplicate provider capability")
			}
			seen[capability] = true
			switch capability {
			case "builds", "queue", "deployments", "metrics", "logs", "traces", "data":
				actual++
			case "history", "sync", "health", "instant", "range", "rules":
			default:
				return nil, errors.New("Unknown provider capability")
			}
		}
		if actual == 0 {
			return nil, errors.New("Provider has no data capabilities")
		}

		var mu sync.Mutex
		retry := map[string]time.Time{}
		definitions = append(definitions, sdk.Definition{Info: info, Create: func(c sdk.Connection, secret string) (sdk.Provider, error) {
			mu.Lock()
			until := retry[c.ID]
			mu.Unlock()
			if time.Now().Before(until) {
				return nil, errors.New("Provider start failed; retry shortly")
			}
			provider, err := launch(path, info, c, secret)
			if err != nil {
				mu.Lock()
				retry[c.ID] = time.Now().Add(3 * time.Second)
				mu.Unlock()
			}
			return provider, err
		}})
	}
	return definitions, nil
}
func validate(root string, m sdk.Manifest) error {
	defs, err := definitions(root, m)
	if err != nil {
		return err
	}
	r, err := platform.NewRegistry()
	if err != nil {
		return err
	}
	return r.RegisterPlugin(m, defs...)
}
func (m *Manager) Load(registry *platform.Registry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	index, err := m.index()
	if err != nil {
		return err
	}
	for id, digest := range index {
		root := filepath.Join(m.root, "packages", digest)
		manifest, e := readManifest(filepath.Join(root, "manifest.json"))
		if e == nil && manifest.ID != id {
			e = errors.New("Plugin ID does not match index")
		}
		if e == nil && m.builtins[id] {
			e = errors.New("Cannot replace built-in plugin")
		}
		if e == nil {
			var defs []sdk.Definition
			defs, e = definitions(root, manifest)
			if e == nil {
				e = registry.RegisterPlugin(manifest, defs...)
			}
		}
		if e != nil {
			m.problems[id] = e.Error()
			continue
		}
		m.loaded[id] = digest
		m.running[id] = manifest
	}
	return nil
}
func (m *Manager) List() ([]Installed, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	index, err := m.index()
	if err != nil {
		return nil, err
	}
	out := []Installed{}
	for id, digest := range index {
		manifest, e := readManifest(filepath.Join(m.root, "packages", digest, "manifest.json"))
		problem := m.problems[id]
		if e != nil {
			manifest.ID = id
			problem = e.Error()
		}
		out = append(out, Installed{Manifest: manifest, PendingRestart: m.loaded[id] != digest, Error: problem})
	}
	for id, manifest := range m.running {
		if _, ok := index[id]; !ok {
			out = append(out, Installed{Manifest: manifest, PendingRestart: true, PendingRemoval: true})
		}
	}
	return out, nil
}
func (m *Manager) Uninstall(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.builtins[id] {
		return errors.New("Built-in plugins cannot be uninstalled")
	}
	index, err := m.index()
	if err != nil {
		return err
	}
	if _, ok := index[id]; !ok {
		return errors.New("Installed plugin not found")
	}
	delete(index, id)
	return m.save(index)
}

// Install only stages files and updates the next-start index. It executes nothing.
func (m *Manager) Install(path string, trusted bool) (sdk.Manifest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var manifest sdk.Manifest
	if !trusted {
		return manifest, errors.New("Only install packages whose native code you trust")
	}
	index, err := m.index()
	if err != nil {
		return manifest, err
	}
	f, err := os.Open(path)
	if err != nil {
		return manifest, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return manifest, err
	}
	if stat.Size() > maxPackage || stat.Size() < 1 {
		return manifest, errors.New("ZIP package exceeds size limit")
	}
	h := sha256.New()
	if _, err = io.Copy(h, io.LimitReader(f, maxPackage+1)); err != nil {
		return manifest, err
	}
	digest := hex.EncodeToString(h.Sum(nil))
	archive, err := zip.NewReader(f, stat.Size())
	if err != nil {
		return manifest, errors.New("Invalid ZIP package")
	}
	if len(archive.File) > 1024 {
		return manifest, errors.New("Too many ZIP entries")
	}
	if err = os.MkdirAll(filepath.Join(m.root, "packages"), 0700); err != nil {
		return manifest, err
	}
	stage, err := os.MkdirTemp(filepath.Join(m.root, "packages"), ".stage-")
	if err != nil {
		return manifest, err
	}
	defer func() {
		rel, e := filepath.Rel(filepath.Join(m.root, "packages"), stage)
		if e == nil && !strings.HasPrefix(rel, "..") && strings.HasPrefix(filepath.Base(stage), ".stage-") {
			_ = os.RemoveAll(stage)
		}
	}()
	seen := map[string]bool{}
	var total uint64
	for _, entry := range archive.File {
		name := entry.Name
		key := strings.ToLower(strings.TrimSuffix(name, "/"))
		if !validPath(name) || seen[key] || entry.Mode()&os.ModeSymlink != 0 || (!entry.FileInfo().IsDir() && !entry.Mode().IsRegular()) {
			return manifest, fmt.Errorf("Unsafe or duplicate ZIP entry: %s", name)
		}
		seen[key] = true
		total += entry.UncompressedSize64
		if total > maxPackage || entry.UncompressedSize64 > 64*1024*1024 {
			return manifest, errors.New("Unpacked package exceeds limit")
		}
		target := filepath.Join(stage, filepath.FromSlash(name))
		rel, e := filepath.Rel(stage, target)
		if e != nil || strings.HasPrefix(rel, "..") {
			return manifest, errors.New("ZIP path escapes package")
		}
		if entry.FileInfo().IsDir() {
			if err = os.MkdirAll(target, 0700); err != nil {
				return manifest, err
			}
			continue
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return manifest, err
		}
		input, e := entry.Open()
		if e != nil {
			return manifest, e
		}
		output, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if e != nil {
			_ = input.Close()
			return manifest, e
		}
		n, copyErr := io.Copy(output, io.LimitReader(input, 64*1024*1024+1))
		inErr := input.Close()
		outErr := output.Close()
		if copyErr != nil || inErr != nil || outErr != nil || uint64(n) != entry.UncompressedSize64 {
			return manifest, errors.New("Cannot extract ZIP entry")
		}
	}
	manifest, err = readManifest(filepath.Join(stage, "manifest.json"))
	if err != nil {
		return manifest, err
	}
	if m.builtins[manifest.ID] {
		return manifest, errors.New("Cannot replace a built-in plugin")
	}
	if err = validate(stage, manifest); err != nil {
		return manifest, err
	}
	// Check provider ownership across the entire next-start package set.
	for id, otherDigest := range index {
		if id == manifest.ID {
			continue
		}
		other, e := readManifest(filepath.Join(m.root, "packages", otherDigest, "manifest.json"))
		if e != nil {
			return manifest, e
		}
		for _, p := range manifest.Providers {
			for _, q := range other.Providers {
				if p.ID == q.ID {
					return manifest, errors.New("Provider ID already belongs to another package")
				}
			}
		}
	}
	for _, p := range manifest.Providers {
		if m.builtins[p.ID] {
			return manifest, errors.New("Cannot replace a built-in provider")
		}
	}
	destination := filepath.Join(m.root, "packages", digest)
	if _, e := os.Stat(destination); os.IsNotExist(e) {
		if err = os.Rename(stage, destination); err != nil {
			return manifest, err
		}
	} else if e != nil {
		return manifest, e
	}
	index[manifest.ID] = digest
	if err = m.save(index); err != nil {
		return manifest, err
	}
	return manifest, nil
}
