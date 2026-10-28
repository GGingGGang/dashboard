package platform

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sort"
	"sync"

	sdk "github.com/GGingGGang/dashboard/sdk"
)

type SourceState struct {
	sdk.SourceDescriptor
	Revision uint64 `json:"revision"`
}

type sourceEntry struct {
	state    SourceState
	ctx      context.Context
	cancel   context.CancelFunc
	resource *sourceResource
	calls    map[uint64]context.CancelFunc
}

type sourceResource struct {
	provider      Provider
	registrations int
	users         int
	closed        bool
}

// Sources is one provider instance's isolated namespace. A provider receives
// only sdk.SourceRegistrar, never the core registry, credentials or storage.
type Sources struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	entries    map[string]*sourceEntry
	resources  map[Provider]*sourceResource
	identities map[string]string
	revision   uint64
	callID     uint64
	closed     bool
}

var _ sdk.SourceRegistrar = (*Sources)(nil)

func NewSources(parent context.Context) *Sources {
	ctx, cancel := context.WithCancel(parent)
	return &Sources{ctx: ctx, cancel: cancel, entries: map[string]*sourceEntry{}, resources: map[Provider]*sourceResource{}, identities: map[string]string{}}
}

func (s *Sources) Register(ctx context.Context, d sdk.SourceDescriptor, p Provider) error {
	return s.replace(ctx, d, p, false)
}
func (s *Sources) Update(ctx context.Context, d sdk.SourceDescriptor, p Provider) error {
	return s.replace(ctx, d, p, true)
}

func (s *Sources) replace(ctx context.Context, d sdk.SourceDescriptor, p Provider, update bool) error {
	if !extensionID.MatchString(d.ID) || d.Name == "" || len(d.Name) > 512 || len(d.TargetID) > 16384 {
		return errors.New("Invalid source descriptor")
	}
	if err := ValidateCapabilities(p, d.Capabilities); err != nil {
		return err
	}
	// Pointer identity permits sharing one implementation without double Close.
	if reflect.TypeOf(p).Kind() != reflect.Pointer {
		return errors.New("Source implementations must use pointer identity")
	}
	s.mu.Lock()
	if err := s.ready(ctx); err != nil {
		s.mu.Unlock()
		return err
	}
	old := s.entries[d.ID]
	if target, seen := s.identities[d.ID]; seen && target != d.TargetID {
		s.mu.Unlock()
		return errors.New("Create a new source ID when changing its target")
	}
	if (old != nil) != update {
		s.mu.Unlock()
		return errors.New("Source already exists or is missing")
	}
	resource := s.resources[p]
	if resource == nil {
		resource = &sourceResource{provider: p}
		s.resources[p] = resource
	}
	resource.registrations++
	s.identities[d.ID] = d.TargetID
	d.Capabilities = slices.Clone(d.Capabilities)
	entryCtx, cancel := context.WithCancel(s.ctx)
	s.revision++
	s.entries[d.ID] = &sourceEntry{state: SourceState{SourceDescriptor: d, Revision: s.revision}, ctx: entryCtx, cancel: cancel, resource: resource, calls: map[uint64]context.CancelFunc{}}
	var closeProvider Provider
	if old != nil {
		cancelEntry(old)
		old.resource.registrations--
		closeProvider = s.retire(old.resource)
	}
	s.mu.Unlock()
	if closeProvider != nil {
		_ = CloseProvider(closeProvider)
	}
	return nil
}

func (s *Sources) ready(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.closed || s.ctx.Err() != nil {
		return errors.New("Source scope is closed")
	}
	return nil
}

func (s *Sources) retire(r *sourceResource) Provider {
	if !r.closed && r.registrations == 0 && r.users == 0 {
		r.closed = true
		delete(s.resources, r.provider)
		return r.provider
	}
	return nil
}

func cancelEntry(e *sourceEntry) {
	e.cancel()
	// Synchronous cancellation makes a late result stale before Update returns.
	for _, cancel := range e.calls {
		cancel()
	}
}

func (s *Sources) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	if err := s.ready(ctx); err != nil {
		s.mu.Unlock()
		return err
	}
	e := s.entries[id]
	if e == nil {
		s.mu.Unlock()
		return errors.New("Source not found")
	}
	delete(s.entries, id)
	cancelEntry(e)
	e.resource.registrations--
	p := s.retire(e.resource)
	s.mu.Unlock()
	if p != nil {
		_ = CloseProvider(p)
	}
	return nil
}

func (s *Sources) List() []SourceState {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SourceState, 0, len(s.entries))
	for _, e := range s.entries {
		d := e.state
		d.Capabilities = slices.Clone(d.Capabilities)
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Acquire holds the implementation alive until release. Callers must discard
// results if the returned context is canceled by source update or deletion.
func (s *Sources) Acquire(ctx context.Context, id string) (context.Context, Provider, uint64, func(), error) {
	s.mu.Lock()
	if err := s.ready(ctx); err != nil {
		s.mu.Unlock()
		return nil, nil, 0, nil, err
	}
	e := s.entries[id]
	if e == nil {
		s.mu.Unlock()
		return nil, nil, 0, nil, errors.New("Source not found")
	}
	e.resource.users++
	callCtx, cancel := context.WithCancel(ctx)
	s.callID++
	callID := s.callID
	e.calls[callID] = cancel
	stop := context.AfterFunc(e.ctx, cancel)
	if e.ctx.Err() != nil {
		cancel()
	}
	s.mu.Unlock()
	var once sync.Once
	release := func() {
		once.Do(func() {
			stop()
			cancel()
			s.mu.Lock()
			delete(e.calls, callID)
			e.resource.users--
			p := s.retire(e.resource)
			s.mu.Unlock()
			if p != nil {
				_ = CloseProvider(p)
			}
		})
	}
	return callCtx, e.resource.provider, e.state.Revision, release, nil
}

// Close cancels work immediately. Busy implementations close when their final
// lease returns; cancellation is cooperative and never closes under a caller.
func (s *Sources) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.cancel()
	var closing []Provider
	for _, e := range s.entries {
		cancelEntry(e)
		e.resource.registrations--
		if p := s.retire(e.resource); p != nil {
			closing = append(closing, p)
		}
	}
	s.entries = map[string]*sourceEntry{}
	s.mu.Unlock()
	for _, p := range closing {
		_ = CloseProvider(p)
	}
}
