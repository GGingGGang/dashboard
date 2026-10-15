package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/zalando/go-keyring"

	"idp-dashboard/internal/application"
	"idp-dashboard/internal/demo"
	"idp-dashboard/internal/jsonsource"
	"idp-dashboard/internal/platform"
	"idp-dashboard/internal/providers"
	"idp-dashboard/internal/storage"
)

type App struct {
	ctx       context.Context
	service   *application.Service
	registry  *platform.Registry
	demoMode  bool
	closeDemo func()
	err       string
}
type ConnectionInput = application.ConnectionInput
type ConnectionView = application.ConnectionView
type State = application.State
type windowsCredentials struct{}

func (windowsCredentials) Get(ref string) (string, error) { return keyring.Get("IDPDashboard", ref) }
func (windowsCredentials) Set(ref, value string) error {
	return keyring.Set("IDPDashboard", ref, value)
}
func (windowsCredentials) Delete(ref string) error { return keyring.Delete("IDPDashboard", ref) }
func NewApp(demoMode bool, extensions ...platform.Definition) *App {
	registry, err := platform.NewRegistry(append(providers.BuiltinDefinitions(), extensions...)...)
	if err != nil {
		panic(err)
	}
	return &App{registry: registry, demoMode: demoMode}
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
	store, err := storage.OpenStore(filepath.Join(root, "dashboard.db"))
	if err != nil {
		a.err = "Cannot open the local database. Existing files have not been reset."
		return
	}
	if a.demoMode {
		base, closeFn := demo.Start()
		a.closeDemo = closeFn
		for _, c := range demo.Connections(base) {
			if err = store.SaveConnection(c, ""); err != nil {
				_ = store.Close()
				a.err = "Cannot prepare demo database"
				return
			}
		}
	}
	a.service = application.New(ctx, store, windowsCredentials{}, a.registry, a.demoMode, jsonsource.Reader{})
	if err := a.service.Start(); err != nil {
		a.err = err.Error()
	}
}

func (a *App) shutdown(context.Context) {
	if a.service != nil {
		_ = a.service.Close()
	}
	if a.closeDemo != nil {
		a.closeDemo()
	}
}
func (a *App) ready() error {
	if a.service == nil {
		return errors.New("Local database is unavailable")
	}
	return nil
}
func (a *App) GetState() State {
	if a.service != nil {
		return a.service.GetState()
	}
	return State{Connections: []ConnectionView{}, Snapshots: []platform.Snapshot{}, Providers: a.registry.Infos(), Error: a.err, Demo: a.demoMode}
}
func (a *App) Refresh() {
	if a.service != nil {
		a.service.Refresh()
	}
}
func (a *App) ReadJSON(request platform.JSONRequest) (platform.JSONResult, error) {
	if err := a.ready(); err != nil {
		return platform.JSONResult{}, err
	}
	return a.service.ReadJSON(request)
}
func (a *App) SaveConnection(input ConnectionInput) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.service.SaveConnection(input)
}
func (a *App) DeleteConnection(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.service.DeleteConnection(id)
}
func (a *App) TestConnection(input ConnectionInput) (application.ConnectionTest, error) {
	if err := a.ready(); err != nil {
		return application.ConnectionTest{}, err
	}
	return a.service.TestConnection(input)
}
func (a *App) Query(id string, q platform.Query) (platform.QueryResult, error) {
	if err := a.ready(); err != nil {
		return platform.QueryResult{}, err
	}
	return a.service.Query(id, q)
}
func (a *App) Presets(kind string) []platform.Rule {
	if a.service != nil {
		return a.service.Presets(kind)
	}
	return []platform.Rule{}
}

func (a *App) SearchLogs(id string, q platform.SearchRequest) (platform.LogResult, error) {
	if err := a.ready(); err != nil {
		return platform.LogResult{}, err
	}
	return a.service.SearchLogs(id, q)
}
func (a *App) SearchTraces(id string, q platform.SearchRequest) (platform.TraceSearchResult, error) {
	if err := a.ready(); err != nil {
		return platform.TraceSearchResult{}, err
	}
	return a.service.SearchTraces(id, q)
}
func (a *App) Trace(id, traceID string) (platform.TraceResult, error) {
	if err := a.ready(); err != nil {
		return platform.TraceResult{}, err
	}
	return a.service.Trace(id, traceID)
}
func (a *App) History(f platform.HistoryFilter) (platform.HistoryPage, error) {
	if err := a.ready(); err != nil {
		return platform.HistoryPage{}, err
	}
	return a.service.History(f)
}
func (a *App) GetPreference(key string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.service.GetPreference(key)
}
func (a *App) SavePreference(key, value string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.service.SavePreference(key, value)
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
	if err = a.service.Backup(path); err != nil {
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
	if a.service != nil {
		w, h := runtime.WindowGetSize(a.ctx)
		if w >= 480 && h >= 600 {
			b, _ := json.Marshal(windowGeometry{w, h})
			_ = a.service.SavePreference("window", string(b))
		}
	}
	return false
}
func (a *App) restoreWindow(ctx context.Context) {
	if a.service == nil {
		return
	}
	raw, err := a.service.GetPreference("window")
	if err != nil {
		return
	}
	var g windowGeometry
	if json.Unmarshal([]byte(raw), &g) == nil && g.Width >= 480 && g.Height >= 600 {
		runtime.WindowSetSize(ctx, min(g.Width, 1920), min(g.Height, 1200))
	}
}
