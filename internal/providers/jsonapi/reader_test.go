package jsonapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platform "github.com/GGingGGang/dashboard/sdk"
	"github.com/itchyny/gojq"
)

func TestPublicHTTPAndJQ(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("unexpected method or credentials")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"service":{"file":{"name":123123}},"items":[{"cost":2},{"cost":3}],"large":9007199254740993,"text":"<img src=x>"}`))
	}))
	defer server.Close()
	for _, tc := range []struct {
		filter, expected string
		scalar           bool
		count            int
	}{
		{`.service.file.name`, "123123", true, 1},
		{`[.items[].cost] | add`, "5", true, 1},
		{`.large`, "9007199254740993", true, 1},
		{`.text`, "<img src=x>", true, 1},
		{`.missing`, "null", true, 1},
		{`.items[].cost`, "[\n  2,\n  3\n]", false, 2},
		{`empty`, "[]", false, 0},
	} {
		t.Run(tc.filter, func(t *testing.T) {
			result, err := (Reader{}).Read(context.Background(), platform.JSONRequest{Source: "http", URL: server.URL, Filter: tc.filter})
			if err != nil {
				t.Fatal(err)
			}
			if result.Text != tc.expected || result.Scalar != tc.scalar || result.Count != tc.count || result.ObservedAt.IsZero() {
				t.Fatalf("unexpected result: %+v", result)
			}
		})
	}
	before := calls
	for _, request := range []platform.JSONRequest{
		{Source: "http", URL: server.URL, Filter: ".["},
		{Source: "command", URL: server.URL, Filter: "."},
		{Source: "http", URL: strings.Replace(server.URL, "://", "://user:secret@", 1), Filter: "."},
	} {
		if _, err := (Reader{}).Read(context.Background(), request); err == nil {
			t.Fatal("expected rejection")
		}
	}
	if calls != before {
		t.Fatal("invalid input caused a request")
	}
}

func TestHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"redirect", "", 302}, {"unauthorized", "secret", 401},
		{"not json", "<html>", 200}, {"extra JSON", "{} {}", 200},
		{"too large", strings.Repeat(" ", maxInput+1), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			if _, err := (Reader{}).Read(context.Background(), platform.JSONRequest{Source: "http", URL: server.URL, Filter: "."}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestQueryLimitsAndCancellation(t *testing.T) {
	for _, filter := range []string{`range(201)`, `"a" * 1048577`, `error("failed")`, `env`} {
		query, err := gojq.Parse(filter)
		if err != nil {
			t.Fatal(err)
		}
		code, err := gojq.Compile(query)
		if err != nil {
			t.Fatal(err)
		}
		result, err := evaluate(context.Background(), code, []byte(`{}`))
		if filter == "env" {
			if err != nil || result.JSON != "{}" {
				t.Fatalf("environment must not be exposed: %+v %v", result, err)
			}
		} else if err == nil {
			t.Fatalf("expected error for %s", filter)
		}
	}
	query, _ := gojq.Parse(`def spin: spin; spin`)
	code, err := gojq.Compile(query)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := evaluate(ctx, code, []byte(`{}`)); err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestConfiguredJSONSourcePreservesIDsAndScalarMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("credentials sent")
		}
		_, _ = w.Write([]byte(`[{"id":9223372036854775807,"count":42}]`))
	}))
	defer server.Close()
	p := &Provider{url: server.URL}
	result, err := p.ReadData(context.Background(), map[string]any{"filter": ".", "mode": "table"})
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range result.Columns {
		if c.Name == "id" && result.Rows[0][i] != "9223372036854775807" {
			t.Fatal("integer precision lost", result)
		}
	}
	scalar, err := p.ReadData(context.Background(), map[string]any{"filter": "length", "mode": "scalar"})
	if err != nil || scalar.Value != "1" {
		t.Fatal(scalar, err)
	}
	if _, err = p.ReadData(context.Background(), map[string]any{"filter": ".", "mode": "scalar"}); err == nil {
		t.Fatal("object accepted as scalar")
	}
	if _, err = p.ReadData(context.Background(), map[string]any{"filter": ".", "path": "https://other.invalid"}); err == nil {
		t.Fatal("absolute path accepted")
	}
}
