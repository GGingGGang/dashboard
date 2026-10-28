package sdk

import (
	"errors"
	"net/url"
	"strings"
)

// ValidateConnection checks provider-independent connection fields.
func ValidateConnection(c Connection) error {
	if c.SchemaVersion < 0 || c.SchemaVersion > 1 {
		return errors.New("Connection schema is newer than this app supports")
	}
	if len(c.Tenant) > 150 || strings.ContainsAny(c.Tenant, "\r\n|") {
		return errors.New("Use one tenant ID of at most 150 characters")
	}
	for _, char := range c.Tenant {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("!-_.*'()", char)) {
			return errors.New("Tenant ID contains unsupported characters")
		}
	}
	if c.Tenant == "." || c.Tenant == ".." {
		return errors.New("Reserved tenant ID")
	}
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
	if c.Auth == "basic" && c.Username == "" {
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
