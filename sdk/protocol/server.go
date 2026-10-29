package protocol

import (
	"context"
	"encoding/json"
	"errors"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"io"
	"reflect"
	"slices"
	"strconv"
	"sync"
)

type resource struct {
	provider sdk.Provider
	users    int
	retired  bool
}
type server struct {
	peer         *Peer
	definitions  []sdk.Definition
	mu           sync.Mutex
	resources    map[string]*resource
	handles      map[sdk.Provider]string
	next         uint64
	initializing bool
	ready        bool
}

// Serve runs one configured provider per process. It returns when the host closes
// stdin. Provider implementations import only the SDK and call this from main.
func Serve(input io.ReadCloser, output io.WriteCloser, definitions ...sdk.Definition) error {
	s := &server{definitions: definitions, resources: map[string]*resource{}, handles: map[sdk.Provider]string{}}
	ready := make(chan struct{})
	s.peer = New(input, output, func(ctx context.Context, method string, raw json.RawMessage) (any, error) {
		<-ready
		return s.handle(ctx, method, raw)
	})
	close(ready)
	<-s.peer.Done()
	s.mu.Lock()
	toClose := []sdk.Provider{}
	for _, r := range s.resources {
		r.retired = true
		if r.users == 0 {
			toClose = append(toClose, r.provider)
		}
	}
	s.resources = map[string]*resource{}
	s.mu.Unlock()
	for _, p := range toClose {
		_ = sdk.CloseProvider(p)
	}
	return nil
}
func (s *server) initialize(ctx context.Context, raw json.RawMessage) (any, error) {
	var args Initialize
	if err := Decode(raw, &args); err != nil {
		return nil, err
	}
	if args.Version != Version || args.Kind != args.Connection.Kind {
		return nil, errors.New("Incompatible provider protocol")
	}
	s.mu.Lock()
	if s.initializing {
		s.mu.Unlock()
		return nil, errors.New("Provider already initialized")
	}
	s.initializing = true
	s.mu.Unlock()
	for _, d := range s.definitions {
		if d.Info.Kind != args.Kind {
			continue
		}
		if d.Validate != nil {
			if err := d.Validate(args.Connection); err != nil {
				return nil, err
			}
		}
		p, err := d.Create(args.Connection, args.Secret)
		if err != nil {
			return nil, err
		}
		if p == nil || reflect.ValueOf(p).Kind() != reflect.Pointer || reflect.ValueOf(p).IsNil() {
			return nil, errors.New("Provider must be a non-nil pointer")
		}
		s.mu.Lock()
		s.resources["0"] = &resource{provider: p}
		s.handles[p] = "0"
		s.ready = true
		s.mu.Unlock()
		_, discover := sdk.Resolve(p, "discover").(sdk.TargetDiscoverer)
		return Initialized{Version: Version, Capabilities: Capabilities(p), Discover: discover}, nil
	}
	return nil, errors.New("Provider kind not found")
}
func (s *server) acquire(id string) (sdk.Provider, func(), error) {
	s.mu.Lock()
	r := s.resources[id]
	if r == nil || r.retired {
		s.mu.Unlock()
		return nil, nil, errors.New("Source handle unavailable")
	}
	r.users++
	s.mu.Unlock()
	return r.provider, func() {
		s.mu.Lock()
		r.users--
		closeNow := r.retired && r.users == 0
		s.mu.Unlock()
		if closeNow {
			_ = sdk.CloseProvider(r.provider)
		}
	}, nil
}
func (s *server) retire(id string) {
	s.mu.Lock()
	r := s.resources[id]
	if r == nil {
		s.mu.Unlock()
		return
	}
	delete(s.resources, id)
	delete(s.handles, r.provider)
	r.retired = true
	closeNow := r.users == 0
	s.mu.Unlock()
	if closeNow {
		_ = sdk.CloseProvider(r.provider)
	}
}
func (s *server) handle(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	if method == "initialize" {
		return s.initialize(ctx, raw)
	}
	var r Request
	if err := Decode(raw, &r); err != nil {
		return nil, err
	}
	if method == "release" {
		s.retire(r.Handle)
		return nil, nil
	}
	p, release, err := s.acquire(r.Handle)
	if err != nil {
		return nil, err
	}
	defer release()
	switch method {
	case "check":
		return nil, p.Check(ctx)
	case "publish":
		if publisher, ok := p.(sdk.SourcePublisher); ok {
			return nil, publisher.PublishSources(ctx, s)
		}
		return nil, nil
	case "discover":
		if v, ok := sdk.Resolve(p, "discover").(sdk.TargetDiscoverer); ok {
			return v.Discover(ctx)
		}
	case "builds":
		if v, ok := sdk.Resolve(p, "builds").(sdk.BuildSource); ok {
			items, more, err := v.Builds(ctx, r.Target, r.Offset)
			return Builds{Items: items, More: more}, err
		}
	case "build":
		if v, ok := sdk.Resolve(p, "builds").(sdk.BuildSource); ok {
			return v.Build(ctx, r.Target, r.Number)
		}
	case "queue":
		if v, ok := sdk.Resolve(p, "queue").(sdk.QueueSource); ok {
			return v.Queue(ctx)
		}
	case "deployments":
		if v, ok := sdk.Resolve(p, "deployments").(sdk.DeploymentSource); ok {
			return v.Deployments(ctx)
		}
	case "query":
		if v, ok := sdk.Resolve(p, "metrics").(sdk.MetricSource); ok {
			return v.Query(ctx, r.Query)
		}
	case "logs":
		if v, ok := sdk.Resolve(p, "logs").(sdk.LogSource); ok {
			return v.SearchLogs(ctx, r.Search)
		}
	case "traces":
		if v, ok := sdk.Resolve(p, "traces").(sdk.TraceSource); ok {
			return v.SearchTraces(ctx, r.Search)
		}
	case "trace":
		if v, ok := sdk.Resolve(p, "traces").(sdk.TraceSource); ok {
			return v.Trace(ctx, r.Target)
		}
	case "data":
		if v, ok := sdk.Resolve(p, "data").(sdk.DataSource); ok {
			result, err := v.ReadData(ctx, r.Input)
			if err != nil {
				return nil, err
			}
			return result.CheckedCopy()
		}
	}
	return nil, errors.New("Unsupported source method")
}
func (s *server) change(ctx context.Context, method string, d sdk.SourceDescriptor, p sdk.Provider) error {
	if p == nil || reflect.ValueOf(p).Kind() != reflect.Pointer || reflect.ValueOf(p).IsNil() {
		return errors.New("Source must be a non-nil pointer")
	}
	actual := Capabilities(p)
	declared := slices.Clone(d.Capabilities)
	slices.Sort(actual)
	slices.Sort(declared)
	if !slices.Equal(actual, declared) {
		return errors.New("Source capabilities do not match implementation")
	}
	s.mu.Lock()
	handle, exists := s.handles[p]
	if !exists {
		s.next++
		handle = strconv.FormatUint(s.next, 10)
		s.handles[p] = handle
		s.resources[handle] = &resource{provider: p}
	}
	s.mu.Unlock()
	err := s.peer.Call(ctx, method, SourceChange{Descriptor: d, Handle: handle}, nil)
	if err != nil && !exists {
		s.mu.Lock()
		delete(s.handles, p)
		delete(s.resources, handle)
		s.mu.Unlock()
	}
	return err
}
func (s *server) Register(ctx context.Context, d sdk.SourceDescriptor, p sdk.Provider) error {
	return s.change(ctx, "source.register", d, p)
}
func (s *server) Update(ctx context.Context, d sdk.SourceDescriptor, p sdk.Provider) error {
	return s.change(ctx, "source.update", d, p)
}
func (s *server) Delete(ctx context.Context, id string) error {
	return s.peer.Call(ctx, "source.delete", SourceChange{Descriptor: sdk.SourceDescriptor{ID: id}}, nil)
}
