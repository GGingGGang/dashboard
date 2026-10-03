import type {
  Build,
  Connection,
  QueryResult,
  Rule,
  State,
} from "../shared/types";
import type { Bridge } from "./types";

// Browser preview is isolated from native settings, credentials, and archive.
export function previewBridge(empty = false): Bridge {
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
        ],
        snapshots: connections.map((c) => ({
          connectionId: c.id,
          attempted: new Date().toISOString(),
          lastSuccess: new Date().toISOString(),
          error: "",
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
      const c = structuredClone(connection);
      c.id ||= `preview-${Date.now()}`;
      connections = connections.filter((x) => x.id !== c.id).concat(c);
      return c.id;
    },
    async DeleteConnection(id) {
      connections = connections.filter((c) => c.id !== id);
    },
    async TestConnection({ connection }) {
      if (!connection.url.startsWith("http"))
        throw new Error("Use an HTTP(S) URL");
      return connection.kind === "prometheus" ? [] : structuredClone(targets);
    },
    async Query(_id, q) {
      if (q.expression.includes("invalid"))
        throw new Error("Query or request rejected by upstream");
      return result(q.expression === "up" ? 1 : 42, q.start > 0);
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
  };
}
