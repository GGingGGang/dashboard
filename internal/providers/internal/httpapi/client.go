package httpapi

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

	"idp-dashboard/internal/platform"
)

type Client struct {
	connection platform.Connection
	base       *url.URL
	client     *http.Client
	secret     string
}

func New(c platform.Connection, secret string) (*Client, error) {
	if err := platform.ValidateConnection(c); err != nil {
		return nil, err
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
	a := &Client{connection: c, base: u, secret: secret, client: &http.Client{Timeout: 8 * time.Second, Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	return a, nil
}

func (a *Client) Get(ctx context.Context, path string, q url.Values, out any) error {
	return a.Request(ctx, http.MethodGet, path, q, nil, out)
}

// POST is used only for the explicitly selected Argo CD session login.
func (a *Client) Request(ctx context.Context, method, path string, q url.Values, body []byte, out any) error {
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
		return &platform.APIError{Status: resp.StatusCode, Message: message}
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

func (a *Client) Link(path string) string {
	base := a.connection.BrowserURL
	if base == "" {
		base = a.connection.URL
	}
	return strings.TrimRight(base, "/") + path
}

// WithBearer shares the transport but keeps session credentials on a separate client.
func (a *Client) WithBearer(token string) *Client {
	copy := *a
	copy.connection.Auth = "bearer"
	copy.secret = token
	return &copy
}

func (a *Client) BasePath() string { return a.base.Path }

func (a *Client) Redact(message string) string {
	if a.secret == "" {
		return message
	}
	return strings.ReplaceAll(message, a.secret, "[redacted]")
}
