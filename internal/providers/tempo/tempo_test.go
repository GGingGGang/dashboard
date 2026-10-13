package tempo

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"idp-dashboard/internal/platform"
)

func TestTraceSearchAndOTLPPrecision(t *testing.T) {
	id := "0123456789abcdef0123456789abcdef"
	b, _ := hex.DecodeString(id)
	encoded := base64.StdEncoding.EncodeToString(b)
	for _, shape := range []string{"batches", "resourceSpans"} {
		t.Run(shape, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.Header.Get("X-Scope-OrgID") != "team-b" {
					t.Error("read scope lost")
				}
				if r.URL.Path == "/api/search" {
					if r.URL.Query().Get("q") != "{ true }" || r.URL.Query().Get("limit") != "1" {
						t.Error("search parameters lost")
					}
					fmt.Fprintf(w, `{"traces":[{"traceID":%q,"rootServiceName":"api","rootTraceName":"GET /","startTimeUnixNano":"1700000000123456789","durationMs":1}]}`, id)
					return
				}
				if r.URL.Path != "/api/traces/"+id || r.URL.RawQuery != "" {
					t.Error("trace lookup reused window or unsafe path")
				}
				fmt.Fprintf(w, `{%q:[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"api"}},{"key":"large","value":{"intValue":"9223372036854775807"}}]},"scopeSpans":[{"scope":{"name":"fixture"},"spans":[{"traceId":%q,"spanId":"AQIDBAUGBwg=","parentSpanId":"CAcGBQQDAgE=","name":"GET /","startTimeUnixNano":"1700000000123456789","endTimeUnixNano":"1700000000123456790","attributes":[],"status":{"code":2},"events":[{"name":"event"}]}]}]}]}`, shape, encoded)
			}))
			defer s.Close()
			p, err := Definition().Create(platform.Connection{Name: "Tempo", URL: s.URL, Auth: "none", Tenant: "team-b"}, "")
			if err != nil {
				t.Fatal(err)
			}
			defer platform.CloseProvider(p)
			source := p.(platform.TraceSource)
			search, err := source.SearchTraces(context.Background(), platform.SearchRequest{Expression: "{ true }", Start: 1700000000, End: 1700000001, Limit: 1})
			if err != nil || len(search.Traces) != 1 || search.Observation.Completeness != "unknown" {
				t.Fatalf("search: %+v %v", search, err)
			}
			r, err := source.Trace(context.Background(), id)
			if err != nil || len(r.Spans) != 1 {
				t.Fatalf("trace: %+v %v", r, err)
			}
			span := r.Spans[0]
			if span.TraceID != id || span.SpanID != "0102030405060708" || span.ParentSpanID != "0807060504030201" || span.StartNS != "1700000000123456789" || span.DurationNS != "1" || span.Service != "api" {
				t.Fatalf("lost span semantics: %+v", span)
			}
			wire, _ := json.Marshal(r)
			if !strings.Contains(string(wire), `"9223372036854775807"`) || !strings.Contains(string(wire), `"events"`) {
				t.Fatal("lost attributes/events")
			}
			if _, err = source.Trace(context.Background(), "../../other"); err == nil {
				t.Fatal("invalid trace path accepted")
			}
		})
	}
}
