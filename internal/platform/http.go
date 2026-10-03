package platform

import (
	"bytes"
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

func validateConnection(c Connection) error {
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
	if (c.Auth == "basic" || c.Auth == "argocd-login") && c.Username == "" {
		return errors.New("Username is required for this authentication method")
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

func newAPI(c Connection, secret string) (*api, error) {
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
	return a, nil
}

func (a *api) get(ctx context.Context, path string, q url.Values, out any) error {
	return a.request(ctx, http.MethodGet, path, q, nil, out)
}

// POST is used only for the explicitly selected Argo CD session login.
func (a *api) request(ctx context.Context, method, path string, q url.Values, body []byte, out any) error {
	// Paths are built by adapters, never copied from upstream URLs.
	endpoint := strings.TrimRight(a.base.String(), "/") + path
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return errors.New("Invalid API path")
	}
	parsed.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, method, parsed.String(), bytes.NewReader(body))
	if err != nil {
		return errors.New("Invalid API request")
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
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
