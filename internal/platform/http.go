package platform

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return e.Message }

type api struct {
	connection Connection
	base       *url.URL
	client     *http.Client
	secret     string
}

func Validate(c Connection) error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("Connection name is required")
	}
	for _, raw := range []string{c.URL, c.BrowserURL} {
		if raw == "" && raw == c.BrowserURL && c.URL != "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("Use an HTTP(S) base URL without credentials, query, or fragment")
		}
	}
	if c.Auth != "none" && c.Auth != "basic" && c.Auth != "bearer" {
		return errors.New("Unsupported authentication method")
	}
	if c.Auth == "basic" && c.Username == "" {
		return errors.New("Username is required for Basic authentication")
	}
	if _, ok := registry[c.Kind]; !ok {
		return errors.New("Unknown provider")
	}
	for _, t := range c.Targets {
		if t.ID == "" {
			return errors.New("Target ID is required")
		}
	}
	for _, r := range c.Rules {
		if r.ID == "" || len(r.Expression) > 16384 || strings.TrimSpace(r.Expression) == "" {
			return errors.New("Rules need an ID and a query of at most 16 KiB")
		}
	}
	return nil
}

func New(c Connection, secret string) (Provider, error) {
	if err := Validate(c); err != nil {
		return nil, err
	}
	if c.Auth != "none" && secret == "" {
		return nil, errors.New("Authentication secret is missing")
	}
	u, _ := url.Parse(strings.TrimRight(c.URL, "/"))
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxConnsPerHost = 4
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if c.CAFile != "" {
		pem, err := os.ReadFile(c.CAFile)
		if err != nil {
			return nil, errors.New("Cannot read the CA file")
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("CA file contains no valid certificates")
		}
		tr.TLSClientConfig.RootCAs = roots
	}
	a := &api{connection: c, base: u, secret: secret, client: &http.Client{Timeout: 8 * time.Second, Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	return registry[c.Kind].create(a), nil
}

var registry = map[string]struct {
	info   ProviderInfo
	create func(*api) Provider
}{
	"jenkins":    {ProviderInfo{Kind: "jenkins", Name: "Jenkins", Category: "ci", Capabilities: []string{"builds", "queue", "history"}, DefaultAuth: "basic"}, func(a *api) Provider { return &Jenkins{a} }},
	"argocd":     {ProviderInfo{Kind: "argocd", Name: "Argo CD", Category: "cd", Capabilities: []string{"deployments", "sync", "health"}, DefaultAuth: "bearer"}, func(a *api) Provider { return &ArgoCD{a} }},
	"prometheus": {ProviderInfo{Kind: "prometheus", Name: "Prometheus", Category: "monitoring", Capabilities: []string{"promql", "instant", "range", "rules"}, DefaultAuth: "none"}, func(a *api) Provider { return &Prometheus{a} }},
}

func Providers() []ProviderInfo {
	out := make([]ProviderInfo, 0, len(registry))
	for _, entry := range registry {
		out = append(out, entry.info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (a *api) get(ctx context.Context, path string, q url.Values, out any) error {
	// Paths are built by adapters, never copied from upstream URLs.
	endpoint := strings.TrimRight(a.base.String(), "/") + path
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return errors.New("Invalid API path")
	}
	parsed.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return errors.New("Invalid API request")
	}
	req.Header.Set("Accept", "application/json")
	if a.connection.Auth == "basic" {
		req.SetBasicAuth(a.connection.Username, a.secret)
	}
	if a.connection.Auth == "bearer" {
		req.Header.Set("Authorization", "Bearer "+a.secret)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return errors.New("Request timed out")
		}
		return errors.New("Connection failed: check address, network, proxy, and TLS certificate")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		message := fmt.Sprintf("Upstream returned HTTP %d", resp.StatusCode)
		switch resp.StatusCode {
		case 401, 403:
			message = "Authentication or read permission denied"
		case 404:
			message = "Target or API path not found"
		case 429:
			message = "Upstream rate limit reached"
		case 400, 422:
			message = "Query or request rejected by upstream"
		case 503:
			message = "Upstream unavailable or query timed out"
		}
		return &APIError{Status: resp.StatusCode, Message: message}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024+1))
	if err != nil {
		return errors.New("Cannot read API response")
	}
	if len(b) > 8*1024*1024 {
		return errors.New("API response exceeds 8 MiB; narrow the target or query")
	}
	if err = json.Unmarshal(b, out); err != nil {
		return errors.New("Unexpected API response; check the base URL and API compatibility")
	}
	return nil
}

func (a *api) link(path string) string {
	base := a.connection.BrowserURL
	if base == "" {
		base = a.connection.URL
	}
	return strings.TrimRight(base, "/") + path
}
