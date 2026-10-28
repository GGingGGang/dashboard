package sdk

import (
	"io"
	"maps"
	"slices"
)

// CloseProvider runs only after the owner has finished using the instance.
func CloseProvider(p Provider) error {
	if closer, ok := p.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func CloneTargets(targets []Target) []Target {
	out := slices.Clone(targets)
	for i := range out {
		out[i].Capabilities = slices.Clone(out[i].Capabilities)
	}
	return out
}

func (c Connection) Clone() Connection {
	c.Config = slices.Clone(c.Config)
	c.Targets = CloneTargets(c.Targets)
	c.Rules = slices.Clone(c.Rules)
	return c
}

func (q QueryResult) Clone() QueryResult {
	q.Warnings = slices.Clone(q.Warnings)
	q.Series = slices.Clone(q.Series)
	for i := range q.Series {
		s := &q.Series[i]
		s.Labels = maps.Clone(s.Labels)
		s.Points = slices.Clone(s.Points)
		for j := range s.Points {
			if s.Points[j].Value != nil {
				value := *s.Points[j].Value
				s.Points[j].Value = &value
			}
		}
	}
	return q
}

func (s Snapshot) Clone() Snapshot {
	s.Modules = maps.Clone(s.Modules)
	s.Builds = slices.Clone(s.Builds)
	s.Queue = slices.Clone(s.Queue)
	s.Deployments = slices.Clone(s.Deployments)
	for i := range s.Deployments {
		s.Deployments[i].Revisions = slices.Clone(s.Deployments[i].Revisions)
	}
	s.Rules = slices.Clone(s.Rules)
	for i := range s.Rules {
		s.Rules[i].Result = s.Rules[i].Result.Clone()
	}
	return s
}

func (p ProviderInfo) Clone() ProviderInfo {
	p.Capabilities = slices.Clone(p.Capabilities)
	p.AuthMethods = slices.Clone(p.AuthMethods)
	if p.Query != nil {
		q := *p.Query
		q.Presets = slices.Clone(q.Presets)
		q.Modes = slices.Clone(q.Modes)
		p.Query = &q
	}
	p.Queries = maps.Clone(p.Queries)
	for k, q := range p.Queries {
		q.Presets = slices.Clone(q.Presets)
		q.Modes = slices.Clone(q.Modes)
		p.Queries[k] = q
	}
	return p
}
