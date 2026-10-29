package extensions

import (
	"context"
	"encoding/json"
	"errors"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"github.com/GGingGGang/dashboard/sdk/protocol"
	"io"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

type process struct {
	cmd     *exec.Cmd
	peer    *protocol.Peer
	cancel  context.CancelFunc
	mu      sync.Mutex
	sources map[string]*remote
	host    sdk.SourceRegistrar
	err     error
	secret  string
}
type remote struct {
	process      *process
	handle       string
	capabilities []string
	discover     bool
	once         sync.Once
}

func launch(path string, info sdk.ProviderInfo, c sdk.Connection, secret string) (sdk.Provider, error) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, path)
	hideWindow(cmd)
	cmd.Dir = directory(path)
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		cancel()
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		cancel()
		return nil, errors.New("Cannot start provider executable")
	}
	p := &process{cmd: cmd, cancel: cancel, sources: map[string]*remote{}, secret: secret}
	ready := make(chan struct{})
	p.peer = protocol.New(stdout, stdin, func(ctx context.Context, method string, raw json.RawMessage) (any, error) {
		<-ready
		return p.handle(ctx, method, raw)
	})
	close(ready)
	go func() { _ = cmd.Wait(); p.fail(errors.New("Provider process exited")) }()
	go func() { <-p.peer.Done(); p.fail(errors.New("Provider protocol disconnected")) }()
	initCtx, stop := context.WithTimeout(context.Background(), 8*time.Second)
	defer stop()
	var reply protocol.Initialized
	if err = p.peer.Call(initCtx, "initialize", protocol.Initialize{Version: protocol.Version, Kind: c.Kind, Connection: c, Secret: secret}, &reply); err != nil {
		p.fail(err)
		return nil, errors.New("Provider initialization failed")
	}
	expected := []string{}
	for _, cap := range info.Capabilities {
		if !slices.Contains([]string{"history", "sync", "health", "instant", "range", "rules"}, cap) {
			expected = append(expected, cap)
		}
	}
	slices.Sort(expected)
	actual := slices.Clone(reply.Capabilities)
	slices.Sort(actual)
	if reply.Version != protocol.Version || !slices.Equal(expected, actual) {
		p.fail(errors.New("Provider capability mismatch"))
		return nil, errors.New("Provider protocol or capabilities do not match manifest")
	}
	r := &remote{process: p, handle: "0", capabilities: reply.Capabilities, discover: reply.Discover}
	p.mu.Lock()
	p.sources["0"] = r
	p.mu.Unlock()
	return r, nil
}
func (p *process) fail(err error) {
	p.mu.Lock()
	if p.err == nil {
		p.err = err
	}
	p.mu.Unlock()
	p.cancel()
	_ = p.peer.Close()
}
func (r *remote) Err() error { r.process.mu.Lock(); defer r.process.mu.Unlock(); return r.process.err }
func (r *remote) Capability(cap string) sdk.Provider {
	if slices.Contains(r.capabilities, cap) || (cap == "discover" && r.discover) {
		return r
	}
	return nil
}
func (r *remote) call(ctx context.Context, method string, args protocol.Request, result any) error {
	args.Handle = r.handle
	err := r.process.peer.Call(ctx, method, args, result)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, protocol.ErrClosed) {
			r.process.fail(err)
		}
		message := err.Error()
		if r.process.secret != "" {
			message = strings.ReplaceAll(message, r.process.secret, "[redacted]")
		}
		if len(message) > 1024 {
			message = message[:1024]
		}
		var rpcError *protocol.RPCError
		if errors.As(err, &rpcError) && rpcError.Data != nil && rpcError.Data.Status != 0 {
			return &sdk.APIError{Status: rpcError.Data.Status, Message: message}
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New(message)
	}
	return nil
}
func (r *remote) Close() error {
	r.once.Do(func() {
		p := r.process
		p.mu.Lock()
		delete(p.sources, r.handle)
		last := len(p.sources) == 0
		p.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = r.call(ctx, "release", protocol.Request{}, nil)
		if last {
			p.fail(errors.New("Provider closed"))
		}
	})
	return nil
}
func (r *remote) PublishSources(ctx context.Context, host sdk.SourceRegistrar) error {
	r.process.mu.Lock()
	r.process.host = host
	r.process.mu.Unlock()
	return r.call(ctx, "publish", protocol.Request{}, nil)
}
func (p *process) handle(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	var change protocol.SourceChange
	if err := protocol.Decode(raw, &change); err != nil {
		return nil, err
	}
	p.mu.Lock()
	host := p.host
	p.mu.Unlock()
	if host == nil {
		return nil, errors.New("Source publication is not active")
	}
	if method == "source.delete" {
		return nil, host.Delete(ctx, change.Descriptor.ID)
	}
	if method != "source.register" && method != "source.update" {
		return nil, errors.New("Unsupported provider callback")
	}
	if len(change.Handle) == 0 || len(change.Handle) > 64 {
		return nil, errors.New("Invalid source handle")
	}
	p.mu.Lock()
	r, exists := p.sources[change.Handle]
	if !exists {
		r = &remote{process: p, handle: change.Handle, capabilities: slices.Clone(change.Descriptor.Capabilities)}
		p.sources[change.Handle] = r
	}
	p.mu.Unlock()
	var err error
	if method == "source.register" {
		err = host.Register(ctx, change.Descriptor, r)
	} else {
		err = host.Update(ctx, change.Descriptor, r)
	}
	if err != nil && !exists {
		p.mu.Lock()
		delete(p.sources, change.Handle)
		p.mu.Unlock()
	}
	return nil, err
}
func (r *remote) Check(ctx context.Context) error {
	return r.call(ctx, "check", protocol.Request{}, nil)
}
func (r *remote) Discover(ctx context.Context) (v []sdk.Target, err error) {
	err = r.call(ctx, "discover", protocol.Request{}, &v)
	return
}
func (r *remote) Builds(ctx context.Context, target string, offset int) ([]sdk.Build, bool, error) {
	var v protocol.Builds
	err := r.call(ctx, "builds", protocol.Request{Target: target, Offset: offset}, &v)
	return v.Items, v.More, err
}
func (r *remote) Build(ctx context.Context, target string, number int64) (v sdk.Build, err error) {
	err = r.call(ctx, "build", protocol.Request{Target: target, Number: number}, &v)
	return
}
func (r *remote) Queue(ctx context.Context) (v []sdk.QueueItem, err error) {
	err = r.call(ctx, "queue", protocol.Request{}, &v)
	return
}
func (r *remote) Deployments(ctx context.Context) (v []sdk.Deployment, err error) {
	err = r.call(ctx, "deployments", protocol.Request{}, &v)
	return
}
func (r *remote) Query(ctx context.Context, q sdk.Query) (v sdk.QueryResult, err error) {
	err = r.call(ctx, "query", protocol.Request{Query: q}, &v)
	return
}
func (r *remote) SearchLogs(ctx context.Context, q sdk.SearchRequest) (v sdk.LogResult, err error) {
	err = r.call(ctx, "logs", protocol.Request{Search: q}, &v)
	return
}
func (r *remote) SearchTraces(ctx context.Context, q sdk.SearchRequest) (v sdk.TraceSearchResult, err error) {
	err = r.call(ctx, "traces", protocol.Request{Search: q}, &v)
	return
}
func (r *remote) Trace(ctx context.Context, id string) (v sdk.TraceResult, err error) {
	err = r.call(ctx, "trace", protocol.Request{Target: id}, &v)
	return
}
func (r *remote) ReadData(ctx context.Context, input map[string]any) (v sdk.DataResult, err error) {
	err = r.call(ctx, "data", protocol.Request{Input: input}, &v)
	return
}

func (r *remote) ReadJSON(ctx context.Context, q sdk.JSONRequest) (v sdk.JSONResult, err error) {
	err = r.call(ctx, "json", protocol.Request{JSON: q}, &v)
	return
}
