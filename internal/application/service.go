package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"idp-dashboard/internal/collector"
	"idp-dashboard/internal/platform"
)

type runner struct {
	sourcesMu sync.Mutex
	sources   *platform.Sources
	ctx       context.Context
	revision  string
	cancel    context.CancelFunc
	refresh   chan struct{}
	done      chan struct{}
	gate      chan struct{}
}
type Service struct {
	ctx         context.Context
	store       Store
	registry    *platform.Registry
	mu          sync.Mutex
	editMu      sync.Mutex
	runners     map[string]*runner
	snapshots   map[string]platform.Snapshot
	gate        chan struct{}
	demoMode    bool
	credentials Credentials
	cancel      context.CancelFunc
	operations  sync.WaitGroup
	closed      bool
	jsonReader  JSONReader
}
type ConnectionView struct {
	platform.Connection
	HasSecret bool   `json:"hasSecret"`
	Revision  string `json:"revision"`
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

func New(ctx context.Context, store Store, credentials Credentials, registry *platform.Registry, demo bool, readers ...JSONReader) *Service {
	ctx, cancel := context.WithCancel(ctx)
	service := &Service{ctx: ctx, cancel: cancel, store: store, credentials: credentials, registry: registry, runners: map[string]*runner{}, snapshots: map[string]platform.Snapshot{}, gate: make(chan struct{}, 2), demoMode: demo}
	if len(readers) > 0 {
		service.jsonReader = readers[0]
	}
	return service
}

func (a *Service) ReadJSON(request platform.JSONRequest) (platform.JSONResult, error) {
	ctx, done, _, err := a.begin("", 25*time.Second)
	if err != nil {
		return platform.JSONResult{}, err
	}
	defer done()
	if a.jsonReader == nil {
		return platform.JSONResult{}, errors.New("JSON 조회를 사용할 수 없습니다")
	}
	return a.jsonReader.Read(ctx, request)
}
func (a *Service) Start() error {
	a.editMu.Lock()
	defer a.editMu.Unlock()
	if err := a.ready(); err != nil {
		return err
	}
	if err := a.restorePlugins(); err != nil {
		return err
	}
	connections, err := a.store.Connections()
	if err != nil {
		return errors.New("Cannot read connection settings")
	}
	for _, c := range connections {
		if !a.registry.Enabled(c.Connection.Kind) {
			a.stop(c.Connection.ID)
			a.start(c)
			continue
		}
		if a.runners[c.Connection.ID] == nil {
			a.start(c)
		}
	}
	return nil
}
func (a *Service) Close() error {
	a.editMu.Lock()
	defer a.editMu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	a.cancel()
	for id := range a.runners {
		a.stop(id)
	}
	a.operations.Wait()
	return a.store.Close()
}
func (a *Service) ready() error {
	if a.closed || a.ctx.Err() != nil {
		return errors.New("Application is closed")
	}
	if a.store == nil {
		return errors.New("Local database is unavailable")
	}
	return nil
}
func (a *Service) find(id string) (platform.SavedConnection, error) {
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
func (a *Service) credential(c platform.SavedConnection) (string, error) {
	if c.Connection.Auth == "none" {
		return "", nil
	}
	secret, err := a.credentials.Get(c.SecretRef)
	if err != nil {
		return "", errors.New("Cannot read the Windows credential; re-enter the token")
	}
	return secret, nil
}
func (a *Service) provider(c platform.SavedConnection) (platform.Provider, error) {
	secret, err := a.credential(c)
	if err != nil {
		return nil, err
	}
	return a.registry.New(c.Connection, secret)
}

func (a *Service) start(c platform.SavedConnection) {
	if !a.registry.Enabled(c.Connection.Kind) {
		a.mu.Lock()
		a.snapshots[c.Connection.ID] = platform.Snapshot{ConnectionID: c.Connection.ID, Error: "Plugin is disabled or unavailable"}
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	r := &runner{ctx: ctx, revision: randomID(), cancel: cancel, refresh: make(chan struct{}, 1), done: make(chan struct{}), gate: make(chan struct{}, 1)}
	a.mu.Lock()
	a.runners[c.Connection.ID] = r
	a.mu.Unlock()
	info, _ := a.registry.Info(c.Connection.Kind)
	if info.OnDemand {
		close(r.done)
		return
	}
	go func() {
		defer close(r.done)
		var p platform.Provider
		var err error
		if info.CollectionSource == "" {
			p, err = a.provider(c)
		}
		if err != nil {
			a.mu.Lock()
			if ctx.Err() == nil {
				a.snapshots[c.Connection.ID] = platform.Snapshot{ConnectionID: c.Connection.ID, Attempted: time.Now(), Error: err.Error()}
			}
			a.mu.Unlock()
			return
		}
		if p != nil {
			defer platform.CloseProvider(p)
		}
		poller := collector.NewCollector(c.Connection, p, a.store)
		var sourceRevision uint64
		var lastSources *platform.Sources
		var sourceChanges <-chan struct{}
		info, _ := a.registry.Info(c.Connection.Kind)
		interval := time.Duration(info.PollSeconds) * time.Second
		for {
			pollCtx, stop := context.WithTimeout(ctx, 60*time.Second)
			release, err := a.admit(pollCtx, r.gate)
			if err != nil {
				stop()
				if ctx.Err() != nil {
					return
				}
				continue
			}
			var snap platform.Snapshot
			if info.CollectionSource == "" {
				snap = poller.Poll(pollCtx)
			} else {
				sourceCtx := context.WithValue(pollCtx, runnerKey{}, r)
				sources, sourceErr := a.sourceScope(sourceCtx, c)
				if sourceErr == nil {
					sourceChanges = sources.Changes()
					callCtx, source, revision, releaseSource, acquireErr := sources.Acquire(sourceCtx, info.CollectionSource)
					sourceErr = acquireErr
					if acquireErr == nil {
						if sources != lastSources || revision != sourceRevision {
							lastSources = sources
							poller = collector.NewCollector(c.Connection, source, a.store)
							sourceRevision = revision
						}
						snap = poller.Poll(callCtx)
						sourceErr = callCtx.Err()
						releaseSource()
					}
				}
				if sourceErr != nil {
					snap = platform.Snapshot{ConnectionID: c.Connection.ID, Attempted: time.Now(), Error: sourceErr.Error(), Builds: []platform.Build{}, Queue: []platform.QueueItem{}, Deployments: []platform.Deployment{}, Rules: []platform.RuleResult{}}
				}
			}
			stop()
			release()
			if ctx.Err() != nil {
				return
			}
			a.mu.Lock()
			if ctx.Err() == nil && a.runners[c.Connection.ID] == r {
				a.snapshots[c.Connection.ID] = snap
			}
			a.mu.Unlock()
			timer := time.NewTimer(interval)
			select {
			case <-timer.C:
			case <-sourceChanges:
				timer.Stop()
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

func (a *Service) stop(id string) {
	a.mu.Lock()
	r := a.runners[id]
	delete(a.runners, id)
	a.mu.Unlock()
	if r != nil {
		r.cancel()
		<-r.done
		r.sourcesMu.Lock()
		if r.sources != nil {
			r.sources.Close()
		}
		r.sourcesMu.Unlock()
	}
}
func (a *Service) Refresh() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, r := range a.runners {
		select {
		case r.refresh <- struct{}{}:
		default:
		}
	}
}
func (a *Service) GetState() State {
	a.editMu.Lock()
	defer a.editMu.Unlock()
	s := State{Connections: []ConnectionView{}, Snapshots: []platform.Snapshot{}, Providers: a.registry.Infos(), Demo: a.demoMode}
	if err := a.ready(); err != nil {
		s.Error = err.Error()
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
		s.Connections = append(s.Connections, ConnectionView{Connection: c.Connection, HasSecret: c.SecretRef != "", Revision: a.revision(c.Connection.ID)})
		if snap, ok := a.snapshots[c.Connection.ID]; ok {
			s.Snapshots = append(s.Snapshots, snap.Clone())
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

func (a *Service) SaveConnection(input ConnectionInput) (string, error) {
	a.editMu.Lock()
	defer a.editMu.Unlock()
	if err := a.ready(); err != nil {
		return "", err
	}
	c := input.Connection.Clone()
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
		if old.Connection.Kind != c.Kind || old.Connection.URL != c.URL || old.Connection.Tenant != c.Tenant {
			return "", errors.New("Create a new connection when changing provider, API address or tenant to keep archives separate")
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
		if err := a.credentials.Set(ref, input.Secret); err != nil {
			return "", errors.New("Cannot save the credential in Windows Credential Manager")
		}
	} else if ref == "" {
		return "", errors.New("Enter the authentication token or password")
	}
	a.stop(c.ID)
	if err := a.store.SaveConnection(c, ref); err != nil {
		if ref != "" && ref != old.SecretRef {
			_ = a.credentials.Delete(ref)
		}
		if old.Connection.ID != "" {
			a.start(old)
		}
		return "", errors.New("Cannot save the connection")
	}
	if old.SecretRef != "" && old.SecretRef != ref {
		_ = a.credentials.Delete(old.SecretRef)
	}
	a.mu.Lock()
	delete(a.snapshots, c.ID)
	a.mu.Unlock()
	a.start(platform.SavedConnection{Connection: c, SecretRef: ref})
	return c.ID, nil
}

func (a *Service) DeleteConnection(id string) error {
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
		_ = a.credentials.Delete(c.SecretRef)
	}
	a.mu.Lock()
	delete(a.snapshots, id)
	a.mu.Unlock()
	return nil
}

type ConnectionTest struct {
	Targets   []platform.Target `json:"targets"`
	Discovery string            `json:"discovery"`
	Warning   string            `json:"warning"`
}

func (a *Service) TestConnection(input ConnectionInput) (result ConnectionTest, err error) {
	ctx, done, saved, err := a.begin(input.Connection.ID, 45*time.Second)
	if err != nil {
		return ConnectionTest{}, err
	}
	defer done()
	defer func() {
		if ctx.Err() != nil {
			result = ConnectionTest{}
			err = ctx.Err()
		}
	}()
	secret := input.Secret
	if secret == "" && input.Connection.ID != "" && input.Connection.Auth != "none" {
		if saved.Connection.URL != input.Connection.URL || saved.Connection.Kind != input.Connection.Kind || saved.Connection.Tenant != input.Connection.Tenant {
			return ConnectionTest{}, errors.New("Enter a new credential when testing another address")
		}
		if saved.Connection.Auth != input.Connection.Auth || ((input.Connection.Auth == "basic" || input.Connection.Auth == "argocd-login") && saved.Connection.Username != input.Connection.Username) {
			return ConnectionTest{}, errors.New("인증 방식이나 사용자명을 바꿀 때는 새 토큰 또는 비밀번호를 입력하세요")
		}
		secret, err = a.credential(saved)
		if err != nil {
			return ConnectionTest{}, err
		}
	}
	p, err := a.registry.New(input.Connection, secret)
	if err != nil {
		return ConnectionTest{}, err
	}
	defer platform.CloseProvider(p)
	if err = p.Check(ctx); err != nil {
		return ConnectionTest{}, err
	}
	result = ConnectionTest{Targets: []platform.Target{}, Discovery: "unsupported"}
	if discovery, ok := platform.Resolve(p, "discover").(platform.TargetDiscoverer); ok {
		targets, discoveryErr := discovery.Discover(ctx)
		if discoveryErr != nil {
			result.Discovery = "failed"
			result.Warning = discoveryErr.Error()
		} else {
			result.Discovery = "complete"
			if targets != nil {
				result.Targets = platform.CloneTargets(targets)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return ConnectionTest{}, err
	}
	return result, nil
}
func (a *Service) Query(id string, q platform.Query) (result platform.QueryResult, err error) {
	if id == "" {
		return platform.QueryResult{}, errors.New("Connection is required")
	}
	ctx, done, c, err := a.begin(id, 8*time.Second)
	if err != nil {
		return platform.QueryResult{}, err
	}
	defer done()
	defer func() {
		if ctx.Err() != nil {
			result = platform.QueryResult{}
			err = ctx.Err()
		}
	}()
	p, err := a.provider(c)
	if err != nil {
		return platform.QueryResult{}, err
	}
	defer platform.CloseProvider(p)
	m, ok := platform.Resolve(p, "metrics").(platform.MetricSource)
	if !ok {
		return platform.QueryResult{}, errors.New("This provider does not support metrics")
	}
	result, err = m.Query(ctx, q)
	if ctx.Err() != nil {
		return platform.QueryResult{}, ctx.Err()
	}
	return result.Clone(), err
}

// begin includes queue time in the deadline and registers work before shutdown can wait.
func (a *Service) begin(id string, timeout time.Duration) (context.Context, func(), platform.SavedConnection, error) {
	a.editMu.Lock()
	if err := a.ready(); err != nil {
		a.editMu.Unlock()
		return nil, nil, platform.SavedConnection{}, err
	}
	parent := a.ctx
	var connectionGate chan struct{}
	var c platform.SavedConnection
	if id != "" {
		var err error
		c, err = a.find(id)
		if err != nil {
			a.editMu.Unlock()
			return nil, nil, c, err
		}
		if !a.registry.Enabled(c.Connection.Kind) {
			a.editMu.Unlock()
			return nil, nil, c, errors.New("Plugin is disabled or unavailable")
		}
		r := a.runners[id]
		if r == nil {
			a.start(c)
			r = a.runners[id]
		}
		parent = context.WithValue(context.WithValue(r.ctx, revisionKey{}, r.revision), runnerKey{}, r)
		connectionGate = r.gate
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	a.operations.Add(1)
	a.editMu.Unlock()
	release, err := a.admit(ctx, connectionGate)
	if err != nil {
		cancel()
		a.operations.Done()
		return nil, nil, c, err
	}
	return ctx, func() { cancel(); release(); a.operations.Done() }, c, nil
}

// Per-connection admission precedes the global gate so a busy connection cannot
// occupy both global slots while one of its calls waits for another.
func (a *Service) admit(ctx context.Context, connectionGate chan struct{}) (func(), error) {
	releaseConnection := func() {}
	if connectionGate != nil {
		select {
		case connectionGate <- struct{}{}:
			releaseConnection = func() { <-connectionGate }
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	select {
	case a.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-a.gate
			releaseConnection()
			return nil, err
		}
		return func() { <-a.gate; releaseConnection() }, nil
	case <-ctx.Done():
		releaseConnection()
		return nil, ctx.Err()
	}
}
func (a *Service) revision(id string) string {
	if r := a.runners[id]; r != nil {
		return r.revision
	}
	return ""
}
func (a *Service) Presets(kind string) []platform.Rule {
	info, ok := a.registry.Info(kind)
	if ok && info.Query != nil {
		return info.Query.Presets
	}
	return []platform.Rule{}
}
func (a *Service) History(f platform.HistoryFilter) (platform.HistoryPage, error) {
	a.editMu.Lock()
	defer a.editMu.Unlock()
	if err := a.ready(); err != nil {
		return platform.HistoryPage{}, err
	}
	p, err := a.store.History(f)
	if err != nil {
		return p, errors.New("Cannot read the local archive")
	}
	return p, nil
}
func (a *Service) GetPreference(key string) (string, error) {
	a.editMu.Lock()
	defer a.editMu.Unlock()
	if err := a.ready(); err != nil {
		return "", err
	}
	return a.store.Preference(key)
}
func (a *Service) SavePreference(key, value string) error {
	a.editMu.Lock()
	defer a.editMu.Unlock()
	if err := a.ready(); err != nil {
		return err
	}
	return a.store.SavePreference(key, value)
}
func (a *Service) Backup(path string) error {
	a.editMu.Lock()
	defer a.editMu.Unlock()
	if err := a.ready(); err != nil {
		return err
	}
	return a.store.Backup(path)
}
