package protocol

import (
	"context"
	"encoding/json"
	sdk "github.com/GGingGGang/dashboard/sdk"
	"io"
	"strings"
	"testing"
	"time"
)

func TestDuplexCallsAndCancellation(t *testing.T) {
	ar, aw := io.Pipe()
	br, bw := io.Pipe()
	cancelled := make(chan struct{})
	a := New(ar, bw, func(ctx context.Context, m string, p json.RawMessage) (any, error) {
		if m == "wait" {
			<-ctx.Done()
			close(cancelled)
			return nil, ctx.Err()
		}
		return "host", nil
	})
	defer a.Close()
	b := New(br, aw, func(context.Context, string, json.RawMessage) (any, error) { return "provider", nil })
	defer b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var result string
	if err := a.Call(ctx, "hello", nil, &result); err != nil || result != "provider" {
		t.Fatal(result, err)
	}
	if err := b.Call(ctx, "hello", nil, &result); err != nil || result != "host" {
		t.Fatal(result, err)
	}
	short, stop := context.WithTimeout(ctx, 20*time.Millisecond)
	defer stop()
	if err := b.Call(short, "wait", nil, nil); err == nil {
		t.Fatal("missing cancellation")
	}
	select {
	case <-cancelled:
	case <-ctx.Done():
		t.Fatal("remote handler not cancelled")
	}
}
func TestMalformedFramesClosePeer(t *testing.T) {
	for _, raw := range []string{"not json\n", `{"jsonrpc":"1.0","id":1}` + "\n", strings.Repeat("x", MaxFrame+2) + "\n"} {
		r, w := io.Pipe()
		outR, outW := io.Pipe()
		p := New(r, outW, nil)
		go func() { _, _ = io.WriteString(w, raw); _ = w.Close() }()
		select {
		case <-p.Done():
		case <-time.After(time.Second):
			t.Fatal("invalid peer remained open")
		}
		_ = outR.Close()
		_ = p.Close()
	}
}

func TestRPCPreservesPartialResultAndUpstreamStatus(t *testing.T) {
	ar, aw := io.Pipe()
	br, bw := io.Pipe()
	a := New(ar, bw, nil)
	defer a.Close()
	b := New(br, aw, func(context.Context, string, json.RawMessage) (any, error) {
		return map[string]string{"partial": "retained"}, &sdk.APIError{Status: 404, Message: "missing"}
	})
	defer b.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var result map[string]string
	err := a.Call(ctx, "read", nil, &result)
	failure, ok := err.(*RPCError)
	if !ok || failure.Data.Status != 404 || result["partial"] != "retained" {
		t.Fatal(result, err)
	}
}
