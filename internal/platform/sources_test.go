package platform_test

import (
	"context"
	"sync/atomic"
	"testing"

	sdk "github.com/GGingGGang/dashboard/sdk"
	"idp-dashboard/internal/platform"
)

type ownedMetric struct{ closed atomic.Int32 }

func (*ownedMetric) Check(context.Context) error { return nil }
func (*ownedMetric) Query(context.Context, platform.Query) (platform.QueryResult, error) {
	return platform.QueryResult{}, nil
}
func (p *ownedMetric) Close() error { p.closed.Add(1); return nil }
func descriptor(id string) sdk.SourceDescriptor {
	return sdk.SourceDescriptor{ID: id, Name: id, Capabilities: []string{"metrics"}}
}

func TestSourceUpdateCancelsBeforeReturningAndDefersClose(t *testing.T) {
	ctx := context.Background()
	s := platform.NewSources(ctx)
	defer s.Close()
	old, next := &ownedMetric{}, &ownedMetric{}
	d := descriptor("metrics")
	if err := s.Register(ctx, d, old); err != nil {
		t.Fatal(err)
	}
	d.Capabilities[0] = "logs"
	if s.List()[0].Capabilities[0] != "metrics" {
		t.Fatal("retained mutable descriptor")
	}
	callCtx, _, revision, release, err := s.Acquire(ctx, "metrics")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(ctx, d, next); err == nil {
		t.Fatal("accepted incompatible implementation")
	}
	if callCtx.Err() != nil || s.List()[0].Revision != revision || old.closed.Load() != 0 {
		t.Fatal("invalid update damaged working source")
	}
	if err := s.Update(ctx, descriptor("metrics"), next); err != nil {
		t.Fatal(err)
	}
	if callCtx.Err() != context.Canceled || old.closed.Load() != 0 {
		t.Fatal("update did not cancel or closed under a caller")
	}
	release()
	release()
	if old.closed.Load() != 1 || s.List()[0].Revision == revision {
		t.Fatal("old implementation leaked or revision reused")
	}
	s.Close()
	if next.closed.Load() != 1 {
		t.Fatal("replacement leaked")
	}
}

func TestSourceNamespacesAndSharedImplementationOwnership(t *testing.T) {
	ctx := context.Background()
	a, b := platform.NewSources(ctx), platform.NewSources(ctx)
	defer a.Close()
	defer b.Close()
	p, other := &ownedMetric{}, &ownedMetric{}
	for _, id := range []string{"one", "two"} {
		if err := a.Register(ctx, descriptor(id), p); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Register(ctx, descriptor("one"), other); err != nil {
		t.Fatal(err)
	}
	if err := a.Delete(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if len(b.List()) != 1 || p.closed.Load() != 0 {
		t.Fatal("cross-owner mutation or shared resource closed")
	}
	callCtx, _, _, release, err := a.Acquire(ctx, "two")
	if err != nil {
		t.Fatal(err)
	}
	a.Close()
	if callCtx.Err() != context.Canceled || p.closed.Load() != 0 {
		t.Fatal("scope close did not cancel safely")
	}
	release()
	if p.closed.Load() != 1 {
		t.Fatal("shared implementation not closed exactly once")
	}
	if err := a.Register(ctx, descriptor("late"), &ownedMetric{}); err == nil {
		t.Fatal("closed registrar accepted mutation")
	}
}

func TestSourceIDCannotBeReassignedToAnotherTarget(t *testing.T) {
	ctx := context.Background()
	s := platform.NewSources(ctx)
	defer s.Close()
	d := descriptor("job")
	d.TargetID = "/job/original/"
	if err := s.Register(ctx, d, &ownedMetric{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	d.TargetID = "/job/unrelated/"
	p := &ownedMetric{}
	if err := s.Register(ctx, d, p); err == nil {
		t.Fatal("reused ID for unrelated target")
	}
	if p.closed.Load() != 0 {
		t.Fatal("host closed rejected caller-owned implementation")
	}
	_ = p.Close()
}
