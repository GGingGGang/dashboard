import { expect, test } from "@playwright/test";

test("dashboard data keeps its periodic refresh through state updates", async ({ page }) => {
  await page.goto("/?empty");
  const result = await page.evaluate(async () => {
    const bridge = await import("/src/bridge/index.ts");
    const { DashboardData } = await import("/src/components/dashboard-data.ts");
    const original = { query: bridge.api.Query, now: Date.now, setTimeout, clearTimeout };
    let now = 1_700_000_000_000;
    let next = 0;
    const timers = new Map<number, { at: number; run: () => void }>();
    let queries = 0;
    (Date as unknown as { now: () => number }).now = () => now;
    (window as unknown as { setTimeout: typeof setTimeout }).setTimeout = ((run: () => void, delay = 0) => {
      const id = ++next; timers.set(id, { at: now + Number(delay), run }); return id as unknown as ReturnType<typeof setTimeout>;
    }) as typeof setTimeout;
    (window as unknown as { clearTimeout: typeof clearTimeout }).clearTimeout = ((id: number) => { timers.delete(id); }) as typeof clearTimeout;
    bridge.api.Query = async () => { queries++; return { type: "matrix", series: [], warnings: [], truncated: false }; };
    const state = {
      demo: true, error: "", snapshots: [],
      connections: [{ id: "metrics", revision: "one", kind: "prometheus", name: "Metrics", url: "http://metrics", browserUrl: "", auth: "none", username: "", caFile: "", targets: [], rules: [] }],
      providers: [{ kind: "prometheus", capabilities: ["metrics"] }],
    } as any;
    const widget = { id: "metric", kind: "metrics", title: "Metric", connectionId: "metrics", targetId: "", expression: "up", unit: "", period: 900, refreshSeconds: 30, x: 0, y: 0, w: 4, h: 3 } as const;
    const data = new DashboardData(() => state, () => {});
    try {
      data.update([widget]);
      await Promise.resolve();
      data.attach(widget.id, document.body);
      for (let seconds = 2; seconds <= 30; seconds += 2) {
        now += 2_000;
        data.update([{ ...widget }]);
        for (const [id, timer] of [...timers]) if (timer.at <= now) { timers.delete(id); timer.run(); }
        await Promise.resolve();
      }
      data.stop();
      return { queries, timers: timers.size };
    } finally {
      data.stop();
      bridge.api.Query = original.query;
      (Date as unknown as { now: () => number }).now = original.now;
      (window as unknown as { setTimeout: typeof setTimeout }).setTimeout = original.setTimeout;
      (window as unknown as { clearTimeout: typeof clearTimeout }).clearTimeout = original.clearTimeout;
    }
  });
  expect(result.queries).toBe(2);
  expect(result.timers).toBe(0);
});

test("dashboard data bounds requests and rejects stale replies after layout changes", async ({ page }) => {
  await page.goto("/?empty");
  const result = await page.evaluate(async () => {
    const bridge = await import("/src/bridge/index.ts");
    const { DashboardData } = await import("/src/components/dashboard-data.ts");
    const original = bridge.api.Query;
    const deferred: { resolve: (value: any) => void; reject: (error: Error) => void }[] = [];
    let inFlight = 0, peak = 0, calls = 0, notified = 0;
    bridge.api.Query = () => new Promise((resolve, reject) => {
      calls++; inFlight++; peak = Math.max(peak, inFlight);
      deferred.push({ resolve: value => { inFlight--; resolve(value); }, reject: error => { inFlight--; reject(error); } });
    });
    const state = {
      demo: true, error: "", snapshots: [],
      connections: [{ id: "metrics", revision: "one", kind: "prometheus", name: "Metrics", url: "http://metrics", browserUrl: "", auth: "none", username: "", caFile: "", targets: [], rules: [] }],
      providers: [{ kind: "prometheus", capabilities: ["metrics"] }],
    } as any;
    const widget = (id: string, expression = "up") => ({ id, kind: "metrics", title: id, connectionId: "metrics", targetId: "", expression, unit: "", period: 900, refreshSeconds: 0, x: 0, y: 0, w: 4, h: 3 }) as const;
    const value = (number: number) => ({ type: "vector", series: [{ labels: {}, points: [{ time: 1, value: number }] }], warnings: [], truncated: false });
    const data = new DashboardData(() => state, () => { notified++; });
    try {
      const a = widget("a"), b = widget("b"), c = widget("c");
      data.update([a, b, c]);
      data.update([a, b]);
      data.update([a, b, c]);
      deferred[0].resolve(value(1));
      await Promise.resolve(); await Promise.resolve();
      const bounded = { peak, callsAfterFirst: calls };
      deferred[1].resolve(value(2));
      await Promise.resolve(); await Promise.resolve();
      deferred[2].resolve(value(3));
      await Promise.resolve(); await Promise.resolve();

      const old = widget("stale", "old");
      data.update([old]);
      const oldRequest = deferred[3];
      state.connections[0].revision = "two";
      data.update([{ ...old, expression: "new" }]);
      oldRequest.reject(new Error("old failure"));
      await Promise.resolve(); await Promise.resolve();
      deferred[4].resolve(value(7));
      await Promise.resolve(); await Promise.resolve();
      const clean = data.markup({ ...old, expression: "new" });

      data.update([{ ...old, expression: "old-success" }]);
      const oldSuccess = deferred[5];
      state.connections[0].revision = "three";
      data.update([{ ...old, expression: "current-failure" }]);
      oldSuccess.resolve(value(99));
      await Promise.resolve(); await Promise.resolve();
      deferred[6].reject(new Error("current failure"));
      await Promise.resolve(); await Promise.resolve();
      const staleSuccess = data.markup({ ...old, expression: "current-failure" });

      data.update([{ ...old, expression: "graph" }]);
      deferred[7].resolve({ type: "matrix", warnings: [], truncated: false, series: [
        { labels: { instance: "one" }, points: [{ time: 1, value: 0.0034 }, { time: 2, value: null }, { time: 3, value: 0.0068 }] },
        { labels: { instance: "two" }, points: [{ time: 2, value: 0.0042 }] },
      ] });
      await Promise.resolve(); await Promise.resolve();
      const graph = data.markup({ ...old, expression: "graph" });

      data.update([{ ...old, expression: "late" }]);
      const late = deferred[8];
      const beforeStop = notified;
      data.stop();
      late.resolve(value(99));
      await Promise.resolve(); await Promise.resolve();
      return { ...bounded, clean, staleSuccess, graph, notifyAfterStop: notified - beforeStop };
    } finally {
      data.stop();
      bridge.api.Query = original;
    }
  });
  expect(result.peak).toBe(2);
  expect(result.callsAfterFirst).toBe(3);
  expect(result.clean).toContain("7");
  expect(result.clean).not.toContain("old failure");
  expect(result.staleSuccess).toContain("current failure");
  expect(result.staleSuccess).not.toContain("99");
  expect(result.graph).toContain("<circle");
  expect(result.graph).toContain("0.0034");
  expect(result.graph).toContain("0.0068");
  expect(result.graph).not.toContain("NaN");
  expect(result.notifyAfterStop).toBe(0);
});
