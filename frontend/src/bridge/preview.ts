import type { FormSchema } from "./extensions";
import type {
  Build,
  Connection,
  QueryResult,
  Rule,
  State,
  Observation,
} from "../shared/types";
import type { Bridge } from "./types";

// Browser preview is isolated from native settings, credentials, and archive.
export function previewBridge(empty = false): Bridge {
  const disabled = new Set<string>();
  const now = Date.now();
  const presets: Rule[] = [
    {
      id: "cpu",
      name: "Node CPU",
      expression:
        '100 * (1 - avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])))',
      threshold: 85,
      unit: "%",
      description: "5-minute average · node-exporter",
      enabled: true,
    },
    {
      id: "memory",
      name: "Node memory",
      expression:
        "100 * (1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)",
      threshold: 90,
      unit: "%",
      description: "Available memory · node-exporter",
      enabled: true,
    },
    {
      id: "disk",
      name: "Filesystem usage",
      expression: "node_filesystem_avail_bytes",
      threshold: 85,
      unit: "%",
      description: "Example data only",
      enabled: true,
    },
  ];
  const targets = [
    {
      id: "/job/payments/",
      name: "payments",
      service: "payments",
      environment: "demo",
    },
    {
      id: "/job/catalog/",
      name: "catalog",
      service: "catalog",
      environment: "demo",
    },
  ];
  const conn = (id: string, kind: string, name: string): Connection => ({
    id,
    kind,
    name,
    url: `http://demo.invalid/${kind}`,
    browserUrl: "",
    auth: "none",
    username: "",
    caFile: "",
    targets:
      kind === "jenkins"
        ? targets
        : kind === "argocd"
          ? targets.map((t) => ({ ...t, id: `argocd/${t.name}` }))
          : [],
    rules: kind === "prometheus" ? presets : [],
  });
  const result = (v: number, range = false): QueryResult => ({
    type: range ? "matrix" : "vector",
    warnings: [],
    truncated: false,
    series: [
      {
        labels: { instance: "demo-node-01" },
        points: Array.from({ length: range ? 60 : 1 }, (_, i) => ({
          time: Math.floor(now / 1000) - (range ? (59 - i) * 60 : 0),
          value: range ? v + Math.sin(i / 4) * 4 : v,
        })),
      },
    ],
  });
  let connections = empty
    ? []
    : [
        conn("ci", "jenkins", "Demo · Jenkins"),
        conn("cd", "argocd", "Demo · Argo CD"),
        conn("metrics", "prometheus", "Demo · Prometheus"),
      ];
  const builds: Build[] = Array.from({ length: 24 }, (_, i) => ({
    connectionId: "ci",
    job: i % 2 ? "/job/catalog/" : "/job/payments/",
    number: 160 - Math.floor(i / 2),
    started: now - 600000 - i * 900000,
    duration: 76000,
    status: i === 0 ? "FAILURE" : i % 7 === 0 ? "UNSTABLE" : "SUCCESS",
    rawStatus: i === 0 ? "FAILURE" : "SUCCESS",
    commit: "d34db33f",
    url: "https://example.org/build",
    observed: now,
  }));
  const preferences = new Map<string, string>();
  const observed = (id: string): Observation => ({ connectionId: id, revision: connections.find(c => c.id === id)?.revision || "preview", requestId: crypto.randomUUID(), observedAt: new Date().toISOString(), windowStart: 0, windowEnd: 0, outcome: "partial", completeness: "unknown", diagnostics: ["브라우저 예시 데이터 · 전체 데이터 여부 미확인"] });
  const traceID = "0123456789abcdef0123456789abcdef";
  return {
    async GetState(): Promise<State> {
      return {
        connections: structuredClone(connections),
        demo: true,
        error: "",
        providers: [
          {
            kind: "jenkins",
            name: "Jenkins",
            category: "ci",
            capabilities: ["builds", "queue", "history"],
            defaultAuth: "basic",
            authMethods: ["basic", "bearer", "none"],
          },
          {
            kind: "argocd",
            name: "Argo CD",
            category: "cd",
            capabilities: ["deployments", "sync", "health"],
            defaultAuth: "bearer",
            authMethods: ["bearer", "argocd-login", "none"],
          },
          {
            kind: "prometheus",
            name: "Prometheus",
            category: "monitoring",
            capabilities: ["metrics", "instant", "range", "rules"],
            defaultAuth: "none",
            query: { language: "PromQL", defaultExpression: "up", presets },
            authMethods: ["none", "basic", "bearer"],
          },
          ...["loki", "tempo"].map(kind => ({ kind, name: kind === "loki" ? "Loki" : "Tempo", category: "observability", capabilities: [kind === "loki" ? "logs" : "traces"], onDemand: true, defaultAuth: "none", authMethods: ["none", "basic", "bearer"], queries: { [kind === "loki" ? "logs" : "traces"]: { language: kind === "loki" ? "LogQL" : "TraceQL", defaultExpression: kind === "loki" ? '{service_name="api"}' : '{ true }', maxResults: kind === "loki" ? 500 : 100 } } })),
        ],
        snapshots: connections.map((c) => ({
          connectionId: c.id,
          attempted: new Date().toISOString(),
          lastSuccess: new Date().toISOString(),
          error: disabled.has(c.kind) ? "Plugin is disabled or unavailable" : "",
          storageError: "",
          imported: 24,
          backfillPending: false,
          builds: c.kind === "jenkins" ? builds : [],
          queue:
            c.kind === "jenkins"
              ? [
                  {
                    id: 7,
                    job: "/job/catalog/",
                    since: now - 120000,
                    reason: "Waiting for an available executor",
                    url: "https://example.org/queue",
                  },
                ]
              : [],
          deployments:
            c.kind === "argocd"
              ? [
                  {
                    id: "argocd/payments",
                    name: "payments",
                    project: "demo",
                    sync: "OutOfSync",
                    health: "Healthy",
                    revisions: ["cafe1234"],
                    phase: "Succeeded",
                    message: "",
                    finished: new Date(now - 3600000).toISOString(),
                    url: "https://example.org/deploy",
                  },
                  {
                    id: "argocd/catalog",
                    name: "catalog",
                    project: "demo",
                    sync: "Synced",
                    health: "Healthy",
                    revisions: ["abcd5678"],
                    phase: "Succeeded",
                    message: "",
                    finished: new Date(now - 3600000).toISOString(),
                    url: "https://example.org/deploy",
                  },
                ]
              : [],
          rules:
            c.kind === "prometheus"
              ? c.rules
                  .filter((r) => r.enabled)
                  .map((r, i) => ({
                    rule: r,
                    result: result([42, 67, 88][i] ?? 1),
                    breaches: i === 2 ? 1 : 0,
                    error: "",
                  }))
              : [],
        })),
      };
    },
    async SaveConnection({ connection }) {
      if (disabled.has(connection.kind)) throw new Error("Plugin is disabled");
      const c = structuredClone(connection);
      c.id ||= `preview-${Date.now()}`;
      c.revision = crypto.randomUUID();
      connections = connections.filter((x) => x.id !== c.id).concat(c);
      return c.id;
    },
    async DeleteConnection(id) {
      connections = connections.filter((c) => c.id !== id);
    },
    async TestConnection({ connection }) {
      if (disabled.has(connection.kind)) throw new Error("Plugin is disabled");
      if (!connection.url.startsWith("http"))
        throw new Error("Use an HTTP(S) URL");
      if (["loki", "tempo"].includes(connection.kind)) return { targets: [], discovery: "unsupported", warning: "" };
      return { targets: connection.kind === "prometheus" ? [] : structuredClone(targets), discovery: "complete", warning: "" };
    },
    async Query(_id, q) {
      if (disabled.has(connections.find(c => c.id === _id)?.kind || "")) throw new Error("Plugin is disabled");
      if (q.expression.includes("invalid"))
        throw new Error("Query or request rejected by upstream");
      return result(q.expression === "up" ? 1 : 42, q.start > 0);
    },
    async ReadJSON() {
      throw new Error("실제 HTTP API 조회는 Windows 앱에서 실행하세요. 브라우저 미리보기에서는 외부 API를 호출하지 않습니다.");
    },
    async SearchLogs(id, q) {
      if (disabled.has(connections.find(c => c.id === id)?.kind || "")) throw new Error("Plugin is disabled");
      return { observation: { ...observed(id), windowStart: q.start, windowEnd: q.end }, entries: [{ timestampNs: String(BigInt(q.end - 1) * 1000000000n + 123456789n), line: "GET /payments 200", labels: { service_name: "payments" }, fields: { trace_id: traceID } }] };
    },
    async SearchTraces(id, q) {
      if (disabled.has(connections.find(c => c.id === id)?.kind || "")) throw new Error("Plugin is disabled");
      return { observation: { ...observed(id), windowStart: q.start, windowEnd: q.end }, traces: [{ traceId: traceID, rootService: "payments", rootName: "GET /payments", startNs: String(BigInt(q.end - 1) * 1000000000n), durationMs: 12 }] };
    },
    async Trace(id, traceId) {
      if (disabled.has(connections.find(c => c.id === id)?.kind || "")) throw new Error("Plugin is disabled");
      return { observation: observed(id), traceId, spans: [{ traceId, spanId: "0102030405060708", parentSpanId: "", name: "GET /payments", service: "payments", startNs: String(BigInt(Date.now()) * 1000000n), durationNs: "12000000", attributes: [], resource: [], details: { status: { code: 1 } }, scope: {} }] };
    },
    async Presets(kind) {
      return kind === "prometheus" ? structuredClone(presets) : [];
    },
    async History(f) {
      const rows = builds.filter(
        (b) =>
          (!f.connectionId || b.connectionId === f.connectionId) &&
          (!f.job || b.job.includes(f.job)) &&
          (!f.status || b.status === f.status) &&
          (!f.since || b.started >= f.since) &&
          (!f.until || b.started <= f.until),
      );
      return {
        builds: rows.slice(f.offset, f.offset + 100),
        total: rows.length,
        size: 65536,
      };
    },
    async GetPreference(k) {
      return preferences.get(k) || "";
    },
    async SavePreference(k, v) {
      preferences.set(k, v);
    },
    async Backup() {
      throw new Error("백업은 Windows 앱에서 사용할 수 있습니다.");
    },
    async OpenLink(url) {
      window.open(url, "_blank", "noopener,noreferrer");
    },
    async PickCA() {
      return "";
    },
    async Refresh() {},
    async Plugins() {
      const state = await this.GetState();
      return state.providers.map(p => ({
        manifest: { id: p.kind, version: "0.1.0", contractVersion: 1,
          providers: [{ id: p.kind, name: p.name, executable: "", configSchema: null }],
          cards: p.kind === "jenkins" ? [
            {id: "builds", title: "Jenkins builds", capability: "builds", renderer: "table" as const, configSchema: {type: "object", properties: {targetId: {type: "string", title: "Job path"}}, required: ["targetId"]} as FormSchema},
            {id: "queue", title: "Jenkins queue", capability: "queue", renderer: "table" as const}] : null,
          dashboards: p.kind === "jenkins" ? [{id: "overview", title: "Jenkins", cards: ["builds", "queue"]}] : null,
          queries: p.kind === "jenkins" ? [{id: "builds", title: "Build lookup", capability: "builds", renderer: "table" as const, inputSchema: {type: "object", properties: {targetId: {type: "string", title: "Job path"}}, required: ["targetId"]} as FormSchema}] : null },
        enabled: !disabled.has(p.kind),
      }));
    },
    async SetPluginEnabled(id, enabled) {
      if (!(await this.Plugins()).some(p => p.manifest.id === id)) throw new Error("Plugin not found");
      if (enabled) disabled.delete(id); else disabled.add(id);
      for (const c of connections) if (c.kind === id) c.revision = crypto.randomUUID();
      preferences.set("plugins.disabled", JSON.stringify([...disabled].sort()));
    },
    async Sources(id) {
      const c = connections.find(c => c.id === id);
      if (!c || disabled.has(c.kind)) throw new Error("Connection or plugin unavailable");
      const p = (await this.GetState()).providers.find(p => p.kind === c.kind);
      const capabilities = (p?.capabilities || []).filter(c => ["builds", "queue", "deployments", "metrics", "logs", "traces"].includes(c));
      return [{ id: "default", name: c.name, capabilities, revision: 1 }];
    },
    async ExecuteContribution(id, sourceId, request) {
      const plugin = (await this.Plugins()).find(p => p.manifest.id === request.pluginId && p.enabled);
      const item = (request.kind === "card" ? plugin?.manifest.cards : plugin?.manifest.queries)?.find(c => c.id === request.id);
      const source = (await this.Sources(id)).find(s => s.id === sourceId);
      if (!item || !source?.capabilities.includes(item.capability)) throw new Error("Plugin contribution unavailable");
      const snapshot = (await this.GetState()).snapshots.find(s => s.connectionId === id);
      const rows = item.capability === "builds" ? (snapshot?.builds || []).filter(b => b.job === request.input.targetId) : snapshot?.queue || [];
      const keys = Object.keys(rows[0] || {});
      return {type: "table", columns: keys.map(name => ({name, type: "string"})), rows: rows.map(row => keys.map(k => String((row as unknown as Record<string, unknown>)[k] ?? "")))};
    },
    async QuerySource(id, sourceId, q) {
      const source = (await this.Sources(id)).find(s => s.id === sourceId);
      if (!source?.capabilities.includes("metrics")) throw new Error("Source does not support metrics");
      return this.Query(id, q);
    },
  };
}
