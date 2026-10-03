// Package demo serves read-only examples on loopback, only when --demo is used.
package demo

import (
	"encoding/json"
	"fmt"
	"idp-dashboard/internal/platform"
	"math"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func Connections(base string) []platform.Connection {
	rules := platform.Presets()
	for i := range rules {
		rules[i].Enabled = true
	}
	return []platform.Connection{
		{ID: "demo-ci", Kind: "jenkins", Name: "Demo · Jenkins", URL: base + "/jenkins", Auth: "none", Targets: []platform.Target{{ID: "/job/payments/", Name: "payments", Service: "payments", Environment: "demo"}, {ID: "/job/catalog/", Name: "catalog", Service: "catalog", Environment: "demo"}}},
		{ID: "demo-cd", Kind: "argocd", Name: "Demo · Argo CD", URL: base + "/argocd", Auth: "none", Targets: []platform.Target{{ID: "argocd/payments", Name: "payments", Service: "payments", Environment: "demo"}, {ID: "argocd/catalog", Name: "catalog", Service: "catalog", Environment: "demo"}}},
		{ID: "demo-metrics", Kind: "prometheus", Name: "Demo · Prometheus", URL: base + "/prometheus", Auth: "none", Rules: rules},
	}
}

func Start() (string, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	base := "http://" + ln.Addr().String()
	epoch := time.Now().Truncate(time.Hour)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		var result any
		switch {
		case r.URL.Path == "/jenkins/api/json":
			result = map[string]any{"mode": "NORMAL", "jobs": []any{map[string]any{"name": "payments", "fullName": "payments", "_class": "org.jenkinsci.plugins.workflow.job.WorkflowJob"}, map[string]any{"name": "catalog", "fullName": "catalog", "_class": "org.jenkinsci.plugins.workflow.job.WorkflowJob"}}}
		case r.URL.Path == "/jenkins/queue/api/json":
			result = map[string]any{"items": []any{map[string]any{"id": 7, "inQueueSince": time.Now().Add(-2 * time.Minute).UnixMilli(), "why": "Waiting for an available executor", "task": map[string]any{"name": "catalog", "url": base + "/jenkins/job/catalog/"}}}}
		case strings.HasPrefix(r.URL.Path, "/jenkins/job/"):
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			job := parts[2]
			build := func(number int) map[string]any {
				status := "SUCCESS"
				if job == "payments" && number == 160 {
					status = "FAILURE"
				}
				if number%9 == 0 {
					status = "UNSTABLE"
				}
				return map[string]any{"number": number, "timestamp": epoch.Add(-time.Duration(161-number) * 10 * time.Minute).UnixMilli(), "duration": 65000 + number*100, "building": false, "result": status, "actions": []any{map[string]any{"lastBuiltRevision": map[string]any{"SHA1": "d34db33f0123456789"}}}}
			}
			if len(parts) > 4 && parts[3] != "api" {
				n, _ := strconv.Atoi(parts[3])
				if n < 1 || n > 160 {
					w.WriteHeader(404)
					return
				}
				result = build(n)
			} else {
				start, end := 0, 100
				match := regexp.MustCompile(`\{(\d+),(\d+)\}`).FindStringSubmatch(r.URL.Query().Get("tree"))
				if len(match) == 3 {
					start, _ = strconv.Atoi(match[1])
					end, _ = strconv.Atoi(match[2])
				}
				rows := []any{}
				for i := start; i < min(end, 160); i++ {
					rows = append(rows, build(160-i))
				}
				result = map[string]any{"builds": rows}
			}
		case r.URL.Path == "/argocd/api/v1/applications":
			apps := []any{}
			for _, name := range []string{"payments", "catalog"} {
				sync := "Synced"
				if name == "payments" {
					sync = "OutOfSync"
				}
				apps = append(apps, map[string]any{"metadata": map[string]string{"name": name, "namespace": "argocd"}, "spec": map[string]string{"project": "demo"}, "status": map[string]any{"sync": map[string]string{"status": sync, "revision": "cafe1234"}, "health": map[string]string{"status": "Healthy"}, "operationState": map[string]string{"phase": "Succeeded", "finishedAt": epoch.Format(time.RFC3339)}}})
			}
			result = map[string]any{"items": apps}
		case strings.HasPrefix(r.URL.Path, "/prometheus/api/v1/query"):
			expr := r.URL.Query().Get("query")
			if strings.Contains(expr, "invalid") {
				w.WriteHeader(422)
				result = map[string]string{"status": "error"}
			} else {
				value := 42.0
				if strings.Contains(expr, "MemAvailable") {
					value = 67
				}
				if strings.Contains(expr, "filesystem") {
					value = 88
				}
				if strings.Contains(expr, "restarts") {
					value = 1
				}
				if expr == "1 - up" {
					value = 0
				}
				if expr == "up" || expr == "vector(1)" {
					value = 1
				}
				typ := "vector"
				row := map[string]any{"metric": map[string]string{"instance": "demo-node-01"}, "value": []any{time.Now().Unix(), fmt.Sprintf("%.2f", value)}}
				if strings.HasSuffix(r.URL.Path, "query_range") {
					typ = "matrix"
					start, _ := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64)
					end, _ := strconv.ParseInt(r.URL.Query().Get("end"), 10, 64)
					step, _ := strconv.ParseInt(r.URL.Query().Get("step"), 10, 64)
					step = max(step, 15)
					values := []any{}
					for t := start; t <= end; t += step {
						values = append(values, []any{t, fmt.Sprintf("%.2f", value+4*math.Sin(float64(t-start)/200))})
					}
					delete(row, "value")
					row["values"] = values
				}
				result = map[string]any{"status": "success", "data": map[string]any{"resultType": typ, "result": []any{row}}}
			}
		default:
			w.WriteHeader(404)
			result = map[string]string{"error": "Demo endpoint not found"}
		}
		_ = json.NewEncoder(w).Encode(result)
	})
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second}
	go srv.Serve(ln)
	return base, func() { _ = srv.Close() }
}
