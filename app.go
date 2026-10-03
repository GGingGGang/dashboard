package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/zalando/go-keyring"
	"idp-dashboard/internal/demo"
	"idp-dashboard/internal/platform"
)

type runner struct {
	cancel  context.CancelFunc
	refresh chan struct{}
	done    chan struct{}
}
type App struct {
	ctx       context.Context
	store     *platform.Store
	registry  *platform.Registry
	mu        sync.Mutex
	editMu    sync.Mutex
	runners   map[string]*runner
	snapshots map[string]platform.Snapshot
	gate      chan struct{}
	err       string
	demoMode  bool
	closeDemo func()
}
type ConnectionView struct {
	platform.Connection
	HasSecret bool `json:"hasSecret"`
}
type State struct {
	Connections []ConnectionView        `json:"connections"`
	Snapshots   []platform.Snapshot     `json:"snapshots"`
	Providers   []platform.ProviderInfo `json:"providers"`
	Error       string                  `json:"error"`
	Demo        bool                    `json:"demo"`
}
type ConnectionInput struct {
	Connection platform.Connection `json:"connection"`
	Secret     string              `json:"secret"`
}

func NewApp(demoMode bool, extensions ...platform.Definition) *App {
	registry, err := platform.NewRegistry(append(platform.BuiltinDefinitions(), extensions...)...)
	if err != nil {
		panic(err)
	}
	return &App{registry: registry, runners: map[string]*runner{}, snapshots: map[string]platform.Snapshot{}, gate: make(chan struct{}, 2), demoMode: demoMode}
}
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		root, _ = os.UserConfigDir()
	}
	root = filepath.Join(root, "IDPDashboard")
	if a.demoMode {
		root = filepath.Join(root, "demo")
	}
	var err error
	a.store, err = platform.OpenStore(filepath.Join(root, "dashboard.db"))
	if err != nil {
		a.err = "Cannot open the local database. Existing files have not been reset."
		return
	}
	if a.demoMode {
		base, closeFn := demo.Start()
		a.closeDemo = closeFn
		for _, c := range demo.Connections(base) {
			if err = a.store.SaveConnection(c, ""); err != nil {
				a.err = "Cannot prepare demo database"
				return
			}
		}
	}
	connections, err := a.store.Connections()
	if err != nil {
		a.err = "Cannot read connection settings"
		return
	}
	for _, c := range connections {
		a.start(c)
	}
}
func (a *App) shutdown(context.Context) {
	a.editMu.Lock()
	defer a.editMu.Unlock()
	a.mu.Lock()
	runs := []*runner{}
	for _, r := range a.runners {
		r.cancel()
		runs = append(runs, r)
	}
	a.mu.Unlock()
	for _, r := range runs {
		<-r.done
	}
	if a.closeDemo != nil {
		a.closeDemo()
	}
	if a.store != nil {
		a.store.Close()
	}
}
func (a *App) ready() error {
	if a.store == nil || a.err != "" {
		return errors.New("Local database is unavailable")
	}
	return nil
}
func (a *App) find(id string) (platform.SavedConnection, error) {
	if err := a.ready(); err != nil {
		return platform.SavedConnection{}, err
	}
	connections, err := a.store.Connections()
	if err != nil {
		return platform.SavedConnection{}, errors.New("Cannot read connections")
	}
	for _, c := range connections {
		if c.Connection.ID == id {
			return c, nil
		}
	}
	return platform.SavedConnection{}, errors.New("Connection not found")
}
func credential(c platform.SavedConnection) (string, error) {
	if c.Connection.Auth == "none" {
		return "", nil
	}
	secret, err := keyring.Get("IDPDashboard", c.SecretRef)
	if err != nil {
		return "", errors.New("Cannot read the Windows credential; re-enter the token")
	}
	return secret, nil
}
func (a *App) provider(c platform.SavedConnection) (platform.Provider, error) {
	secret, err := credential(c)
	if err != nil {
		return nil, err
	}
	return a.registry.New(c.Connection, secret)
}

func (a *App) start(c platform.SavedConnection) {
	ctx, cancel := context.WithCancel(a.ctx)
	r := &runner{cancel: cancel, refresh: make(chan struct{}, 1), done: make(chan struct{})}
	a.mu.Lock()
	a.runners[c.Connection.ID] = r
	a.mu.Unlock()
	go func() {
		defer close(r.done)
		p, err := a.provider(c)
		if err != nil {
			a.mu.Lock()
			a.snapshots[c.Connection.ID] = platform.Snapshot{ConnectionID: c.Connection.ID, Attempted: time.Now(), Error: err.Error()}
			a.mu.Unlock()
			return
		}
		collector := platform.NewCollector(c.Connection, p, a.store)
		info, _ := a.registry.Info(c.Connection.Kind)
		interval := time.Duration(info.PollSeconds) * time.Second
		for {
			select {
			case a.gate <- struct{}{}:
			case <-ctx.Done():
				return
			}
			pollCtx, stop := context.WithTimeout(ctx, 60*time.Second)
			snap := collector.Poll(pollCtx)
			stop()
			<-a.gate
			if ctx.Err() != nil {
				return
			}
			a.mu.Lock()
			a.snapshots[c.Connection.ID] = snap
			a.mu.Unlock()
			timer := time.NewTimer(interval)
			select {
			case <-timer.C:
			case <-r.refresh:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
	}()
}

func (a *App) stop(id string) {
	a.mu.Lock()
	r := a.runners[id]
	delete(a.runners, id)
	a.mu.Unlock()
	if r != nil {
		r.cancel()
		<-r.done
	}
}
func (a *App) Refresh() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, r := range a.runners {
		select {
		case r.refresh <- struct{}{}:
		default:
		}
	}
}
func (a *App) GetState() State {
	s := State{Connections: []ConnectionView{}, Snapshots: []platform.Snapshot{}, Providers: a.registry.Infos(), Error: a.err, Demo: a.demoMode}
	if a.store == nil {
		return s
	}
	cs, err := a.store.Connections()
	if err != nil {
		s.Error = "Cannot read local settings"
		return s
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, c := range cs {
		s.Connections = append(s.Connections, ConnectionView{c.Connection, c.SecretRef != ""})
		if snap, ok := a.snapshots[c.Connection.ID]; ok {
			s.Snapshots = append(s.Snapshots, snap)
		}
	}
	return s
}
func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (a *App) SaveConnection(input ConnectionInput) (string, error) {
	a.editMu.Lock()
	defer a.editMu.Unlock()
	if err := a.ready(); err != nil {
		return "", err
	}
	c := input.Connection
	if err := a.registry.Validate(c); err != nil {
		return "", err
	}
	if len(c.Targets) > 50 || len(c.Rules) > 20 {
		return "", errors.New("Use up to 50 targets and 20 rules per connection")
	}
	old := platform.SavedConnection{}
	if c.ID == "" {
		c.ID = randomID()
	} else {
		var err error
		old, err = a.find(c.ID)
		if err != nil {
			return "", err
		}
		if old.Connection.Kind != c.Kind || old.Connection.URL != c.URL {
			return "", errors.New("Create a new connection when changing provider or API address to keep archives separate")
		}
	}
	ref := old.SecretRef
	if c.Auth != "none" && input.Secret == "" && old.Connection.ID != "" && (old.Connection.Auth != c.Auth || ((c.Auth == "basic" || c.Auth == "argocd-login") && old.Connection.Username != c.Username)) {
		return "", errors.New("인증 방식이나 사용자명을 바꿀 때는 새 토큰 또는 비밀번호를 입력하세요")
	}
	if c.Auth == "none" {
		ref = ""
	} else if input.Secret != "" {
		ref = randomID()
		if err := keyring.Set("IDPDashboard", ref, input.Secret); err != nil {
			return "", errors.New("Cannot save the credential in Windows Credential Manager")
		}
	} else if ref == "" {
		return "", errors.New("Enter the authentication token or password")
	}
	a.stop(c.ID)
	if err := a.store.SaveConnection(c, ref); err != nil {
		if ref != "" && ref != old.SecretRef {
			_ = keyring.Delete("IDPDashboard", ref)
		}
		if old.Connection.ID != "" {
			a.start(old)
		}
		return "", errors.New("Cannot save the connection")
	}
	if old.SecretRef != "" && old.SecretRef != ref {
		_ = keyring.Delete("IDPDashboard", old.SecretRef)
	}
	a.mu.Lock()
	delete(a.snapshots, c.ID)
	a.mu.Unlock()
	a.start(platform.SavedConnection{Connection: c, SecretRef: ref})
	return c.ID, nil
}

func (a *App) DeleteConnection(id string) error {
	a.editMu.Lock()
	defer a.editMu.Unlock()
	c, err := a.find(id)
	if err != nil {
		return err
	}
	a.stop(id)
	if err = a.store.DeleteConnection(id); err != nil {
		a.start(c)
		return errors.New("Cannot remove connection")
	}
	if c.SecretRef != "" {
		_ = keyring.Delete("IDPDashboard", c.SecretRef)
	}
	a.mu.Lock()
	delete(a.snapshots, id)
	a.mu.Unlock()
	return nil
}

func (a *App) TestConnection(input ConnectionInput) ([]platform.Target, error) {
	secret := input.Secret
	if secret == "" && input.Connection.ID != "" && input.Connection.Auth != "none" {
		saved, err := a.find(input.Connection.ID)
		if err != nil {
			return nil, err
		}
		if saved.Connection.URL != input.Connection.URL || saved.Connection.Kind != input.Connection.Kind {
			return nil, errors.New("Enter a new credential when testing another address")
		}
		if saved.Connection.Auth != input.Connection.Auth || ((input.Connection.Auth == "basic" || input.Connection.Auth == "argocd-login") && saved.Connection.Username != input.Connection.Username) {
			return nil, errors.New("인증 방식이나 사용자명을 바꿀 때는 새 토큰 또는 비밀번호를 입력하세요")
		}
		secret, err = credential(saved)
		if err != nil {
			return nil, err
		}
	}
	p, err := a.registry.New(input.Connection, secret)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	defer cancel()
	select {
	case a.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, errors.New("Connection test timed out")
	}
	defer func() { <-a.gate }()
	if err = p.Check(ctx); err != nil {
		return nil, err
	}
	return p.Discover(ctx)
}

func (a *App) Query(id string, q platform.Query) (platform.QueryResult, error) {
	c, err := a.find(id)
	if err != nil {
		return platform.QueryResult{}, err
	}
	p, err := a.provider(c)
	if err != nil {
		return platform.QueryResult{}, err
	}
	m, ok := p.(platform.Monitoring)
	if !ok {
		return platform.QueryResult{}, errors.New("This provider does not support metrics")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
	defer cancel()
	select {
	case a.gate <- struct{}{}:
	case <-ctx.Done():
		return platform.QueryResult{}, errors.New("Query timed out")
	}
	defer func() { <-a.gate }()
	return m.Query(ctx, q)
}
func (a *App) Presets(kind string) []platform.Rule {
	info, ok := a.registry.Info(kind)
	if ok && info.Query != nil {
		return info.Query.Presets
	}
	return []platform.Rule{}
}
func (a *App) History(f platform.HistoryFilter) (platform.HistoryPage, error) {
	if err := a.ready(); err != nil {
		return platform.HistoryPage{}, err
	}
	p, err := a.store.History(f)
	if err != nil {
		return p, errors.New("Cannot read the local archive")
	}
	return p, nil
}
func (a *App) GetPreference(key string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.store.Preference(key)
}
func (a *App) SavePreference(key, value string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.store.SavePreference(key, value)
}
func (a *App) Backup() (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Export archive backup", DefaultFilename: "idp-dashboard-" + time.Now().Format("20060102-150405") + ".db", Filters: []runtime.FileFilter{{DisplayName: "SQLite database", Pattern: "*.db"}}})
	if err != nil {
		return "", errors.New("Cannot open save dialog")
	}
	if path == "" {
		return "", nil
	}
	if err = a.store.Backup(path); err != nil {
		return "", err
	}
	return path, nil
}
func (a *App) OpenLink(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return errors.New("Only HTTP(S) links are allowed")
	}
	runtime.BrowserOpenURL(a.ctx, raw)
	return nil
}
func (a *App) PickCA() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Select trusted CA certificate", Filters: []runtime.FileFilter{{DisplayName: "PEM certificate", Pattern: "*.pem;*.crt"}}})
}

type windowGeometry struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (a *App) beforeClose(context.Context) bool {
	if a.store != nil {
		w, h := runtime.WindowGetSize(a.ctx)
		if w >= 480 && h >= 600 {
			b, _ := json.Marshal(windowGeometry{w, h})
			_ = a.store.SavePreference("window", string(b))
		}
	}
	return false
}
func (a *App) restoreWindow(ctx context.Context) {
	if a.store == nil {
		return
	}
	raw, err := a.store.Preference("window")
	if err != nil {
		return
	}
	var g windowGeometry
	if json.Unmarshal([]byte(raw), &g) == nil && g.Width >= 480 && g.Height >= 600 {
		runtime.WindowSetSize(ctx, min(g.Width, 1920), min(g.Height, 1200))
	}
}
