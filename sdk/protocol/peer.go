// Package protocol implements contract-v1 JSON-RPC 2.0 over newline-delimited stdio.
// Stdout is reserved for RPC; diagnostics belong on stderr.
package protocol

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"io"
	"sync"
	"sync/atomic"
)

const Version = 1
const MaxFrame = 8 * 1024 * 1024

var ErrClosed = errors.New("Provider transport closed")

type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}
type ErrorData struct {
	Status int             `json:"status,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}
type RPCError struct {
	Data    *ErrorData `json:"data,omitempty"`
	Code    int        `json:"code"`
	Message string     `json:"message"`
}

func (e *RPCError) Error() string { return e.Message }

type Handler func(context.Context, string, json.RawMessage) (any, error)
type Peer struct {
	input   io.ReadCloser
	output  io.WriteCloser
	handler Handler
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	pending map[uint64]chan Message
	running map[uint64]context.CancelFunc
	writes  chan []byte
	serial  atomic.Uint64
	once    sync.Once
}

func New(input io.ReadCloser, output io.WriteCloser, handler Handler) *Peer {
	ctx, cancel := context.WithCancel(context.Background())
	p := &Peer{input: input, output: output, handler: handler, ctx: ctx, cancel: cancel, pending: map[uint64]chan Message{}, running: map[uint64]context.CancelFunc{}, writes: make(chan []byte, 32)}
	go p.read()
	go p.write()
	return p
}
func (p *Peer) Done() <-chan struct{} { return p.ctx.Done() }
func (p *Peer) Close() error {
	p.once.Do(func() {
		p.cancel()
		_ = p.input.Close()
		_ = p.output.Close()
		p.mu.Lock()
		for _, cancel := range p.running {
			cancel()
		}
		p.mu.Unlock()
	})
	return nil
}
func (p *Peer) send(ctx context.Context, m Message) error {
	m.JSONRPC = "2.0"
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(raw) > MaxFrame {
		return errors.New("RPC frame exceeds limit")
	}
	raw = append(raw, '\n')
	select {
	case p.writes <- raw:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return ErrClosed
	}
}
func (p *Peer) Call(ctx context.Context, method string, params, result any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	id := p.serial.Add(1)
	ch := make(chan Message, 1)
	p.mu.Lock()
	p.pending[id] = ch
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, id); p.mu.Unlock() }()
	if err = p.send(ctx, Message{ID: id, Method: method, Params: raw}); err != nil {
		return err
	}
	select {
	case m := <-ch:
		if m.Error != nil {
			if result != nil && m.Error.Data != nil && len(m.Error.Data.Result) > 0 {
				if err := json.Unmarshal(m.Error.Data.Result, result); err != nil {
					return err
				}
			}
			return m.Error
		}
		if result != nil {
			return json.Unmarshal(m.Result, result)
		}
		return nil
	case <-ctx.Done():
		cancel, _ := json.Marshal(struct {
			ID uint64 `json:"id"`
		}{id})
		notification, _ := json.Marshal(Message{JSONRPC: "2.0", Method: "$/cancelRequest", Params: cancel})
		select {
		case p.writes <- append(notification, '\n'):
		default:
		}
		return ctx.Err()
	case <-p.ctx.Done():
		return ErrClosed
	}
}
func (p *Peer) write() {
	for {
		select {
		case raw := <-p.writes:
			if _, err := p.output.Write(raw); err != nil {
				_ = p.Close()
				return
			}
		case <-p.ctx.Done():
			return
		}
	}
}
func (p *Peer) read() {
	defer p.Close()
	scanner := bufio.NewScanner(p.input)
	scanner.Buffer(make([]byte, 4096), MaxFrame+1)
	for scanner.Scan() {
		var m Message
		if json.Unmarshal(scanner.Bytes(), &m) != nil || m.JSONRPC != "2.0" {
			return
		}
		if m.Method == "$/cancelRequest" {
			var v struct {
				ID uint64 `json:"id"`
			}
			if json.Unmarshal(m.Params, &v) != nil {
				return
			}
			p.mu.Lock()
			if cancel := p.running[v.ID]; cancel != nil {
				cancel()
			}
			p.mu.Unlock()
			continue
		}
		if m.Method == "" {
			if m.ID == 0 {
				return
			}
			p.mu.Lock()
			ch := p.pending[m.ID]
			p.mu.Unlock()
			if ch != nil {
				select {
				case ch <- m:
				default:
				}
			}
			continue
		}
		if m.ID == 0 {
			return
		}
		ctx, cancel := context.WithCancel(p.ctx)
		p.mu.Lock()
		if len(p.running) >= 32 || p.running[m.ID] != nil {
			p.mu.Unlock()
			cancel()
			return
		}
		p.running[m.ID] = cancel
		p.mu.Unlock()
		go func(m Message) {
			defer cancel()
			defer func() { p.mu.Lock(); delete(p.running, m.ID); p.mu.Unlock() }()
			var value any
			var err error
			if p.handler == nil {
				err = errors.New("Unsupported RPC method")
			} else {
				value, err = p.handler(ctx, m.Method, m.Params)
			}
			response := Message{ID: m.ID}
			if err != nil {
				partial, _ := json.Marshal(value)
				data := &ErrorData{Result: partial}
				var upstream *sdk.APIError
				if errors.As(err, &upstream) {
					data.Status = upstream.Status
				}
				response.Error = &RPCError{Code: -32000, Message: err.Error(), Data: data}
			} else {
				response.Result, err = json.Marshal(value)
				if err != nil {
					response.Error = &RPCError{Code: -32603, Message: "Cannot encode result"}
				}
			}
			if err = p.send(ctx, response); err != nil && ctx.Err() == nil {
				_ = p.Close()
			}
		}(m)
	}
}
func Decode(raw json.RawMessage, out any) error {
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("Invalid RPC parameters: %w", err)
	}
	return nil
}
