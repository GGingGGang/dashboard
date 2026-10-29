import { expect, test, type Page } from "@playwright/test";

const layoutKey = "dashboard.layout";

test.beforeEach(async ({ page }) => {
  await page.addInitScript((key) => {
    const w = window as any;
    const now = new Date().toISOString();
    const connection = (id: string, kind: string, name: string) => ({
      id, kind, name, url: "https://example.invalid", browserUrl: "", auth: "none",
      username: "", caFile: "", targets: [{ id: `${id}/api`, name: "api", service: "api", environment: "demo" }], rules: [],
    });
    const providers = [
      { kind: "jenkins", name: "Jenkins", category: "ci", capabilities: ["builds", "queue"], defaultAuth: "none", authMethods: ["none"] },
      { kind: "argocd", name: "Argo CD", category: "cd", capabilities: ["deployments"], defaultAuth: "none", authMethods: ["none"] },
      { kind: "prometheus", name: "Prometheus", category: "monitoring", capabilities: ["metrics", "instant", "range"], defaultAuth: "none", authMethods: ["none"], query: { language: "PromQL", defaultExpression: "up", presets: [] } },
      { kind: "loki", name: "Loki", category: "observability", capabilities: ["logs"], defaultAuth: "none", authMethods: ["none"] },
      { kind: "tempo", name: "Tempo", category: "observability", capabilities: ["traces"], defaultAuth: "none", authMethods: ["none"] },
    ];
    const result = (value = 1) => ({ type: "vector", warnings: [], truncated: false, series: [{ labels: { source: "fixture" }, points: [{ time: Date.now() / 1000, value }] }] });
    w.dashboardFixture = { removeMetrics: false, failSave: false, queryCalls: 0, calls: [] as any[] };
    w.go = { main: { App: {
      GetPreference: async (name: string) => localStorage.getItem(name) || "",
      SavePreference: async (name: string, value: string) => {
        if (name === key && w.dashboardFixture.failSave) throw new Error("preference unavailable");
        localStorage.setItem(name, value);
      },
      GetState: async () => {
        const connections = [
          connection("ci", "jenkins", "CI"), connection("cd", "argocd", "CD"), connection("metrics", "prometheus", "Metrics"),
          connection("logs", "loki", "Logs"), connection("traces", "tempo", "Traces"),
        ].filter((c) => !(w.dashboardFixture.removeMetrics && c.id === "metrics"));
        return {
          connections, providers, demo: true, error: "", snapshots: connections.map((c: any) => ({
            connectionId: c.id, attempted: now, lastSuccess: now, error: "", storageError: "", imported: 0, backfillPending: false,
            builds: c.id === "ci" ? [{ connectionId: "ci", job: "ci/api", number: 1, started: Date.now(), duration: 10, status: "SUCCESS", rawStatus: "SUCCESS", commit: "abc", url: "", observed: Date.now() }] : [],
            queue: [], deployments: c.id === "cd" ? [{ id: "cd/api", name: "api", project: "demo", sync: "Synced", health: "Healthy", revisions: ["abc"], phase: "Succeeded", message: "", finished: now, url: "" }] : [], rules: [],
          })),
        };
      },
      Query: async (_id: string, query: any) => { w.dashboardFixture.queryCalls++; return result(query.expression === "rate(http_requests_total[5m])" ? 42 : 1); },
      SearchLogs: async (id: string, query: any) => {
        w.dashboardFixture.calls.push({ type: "logs", id, query });
        return { observation: { connectionId: id, revision: "1", requestId: "log", observedAt: now, windowStart: query.start, windowEnd: query.end, outcome: "success", completeness: "complete", diagnostics: [] }, entries: [{ timestampNs: "1700000000123456789", line: "<img src=x onerror=alert(1)> raw log", labels: { service: "api" }, fields: {} }] };
      },
      SearchTraces: async (id: string, query: any) => {
        w.dashboardFixture.calls.push({ type: "traces", id, query });
        return { observation: { connectionId: id, revision: "1", requestId: "trace", observedAt: now, windowStart: query.start, windowEnd: query.end, outcome: "success", completeness: "complete", diagnostics: [] }, traces: [{ traceId: "0123456789abcdef0123456789abcdef", rootService: "api", rootName: "GET /health", startNs: "1700000000123456789", durationMs: 3 }] };
      },
      Trace: async () => ({ observation: {}, spans: [] }),
      SaveConnection: async () => "", DeleteConnection: async () => {}, TestConnection: async () => ({ targets: [], discovery: "unsupported", warning: "" }),
      Presets: async () => [], History: async () => ({ builds: [], total: 0, size: 0 }), Backup: async () => "", OpenLink: async () => {}, PickCA: async () => "", Refresh: async () => {},
    } } };
  }, layoutKey);
  await page.goto("/");
  await page.locator('nav [data-page="dashboard"]').click();
  await expect(page.locator(".dashboard-page")).toBeVisible();
});

async function addMetric(page: Page, title: string, expression: string) {
  await page.locator("#dashboard-add").click();
  const dialog = page.locator("#dashboard-widget-dialog");
  await expect(dialog).toBeVisible();
  await dialog.locator('[name="kind"]').selectOption("metrics");
  await dialog.locator('[name="title"]').fill(title);
  await dialog.locator('[name="expression"]').fill(expression);
  await dialog.locator("#dashboard-widget-form").getByRole("button", { name: "저장", exact: true }).click();
}

test("allows duplicate metric widgets with independently configured queries", async ({ page }) => {
  await page.locator("#dashboard-edit").click();
  await addMetric(page, "요청 수", "up");
  await addMetric(page, "요청 속도", "rate(http_requests_total[5m])");
  const metrics = page.locator('.dashboard-widget[data-widget-id]').filter({ has: page.locator(".dashboard-widget-title", { hasText: /요청/ }) });
  await expect(metrics).toHaveCount(2);
  await expect(metrics.nth(0)).toContainText("요청 수");
  await expect(metrics.nth(1)).toContainText("요청 속도");
  await page.locator("#dashboard-edit").click();
  const saved = await page.evaluate((key) => JSON.parse(localStorage.getItem(key) || "{}"), layoutKey);
  expect(saved.widgets.filter((w: any) => w.kind === "metrics" && /요청/.test(w.title)).map((w: any) => w.expression)).toEqual(["up", "rate(http_requests_total[5m])"]);
});

test("log and trace cards keep independent queries, bounded requests, and escaped log text", async ({ page }) => {
  await page.locator("#dashboard-edit").click();
  for (const [kind, title, expression] of [
    ["logs", "인증 로그", '{service="api"}'],
    ["traces", "인증 트레이스", '{ resource.service.name = "api" }'],
  ]) {
    await page.locator("#dashboard-add").click();
    const dialog = page.locator("#dashboard-widget-dialog");
    await dialog.locator('[name="kind"]').selectOption(kind);
    await dialog.locator('[name="title"]').fill(title);
    await dialog.locator('[name="expression"]').fill(expression);
    await dialog.locator('[name="period"]').selectOption("86400");
    await dialog.locator("#dashboard-widget-form").getByRole("button", { name: "저장", exact: true }).click();
  }
  await page.locator("#dashboard-edit").click();
  await expect.poll(() => page.evaluate(() => (window as any).dashboardFixture.calls.length)).toBeGreaterThanOrEqual(2);
  const calls = await page.evaluate(() => (window as any).dashboardFixture.calls);
  const logs = calls.find((call: any) => call.type === "logs");
  const traces = calls.find((call: any) => call.type === "traces");
  expect(logs).toMatchObject({ id: "logs", query: { expression: '{service="api"}', limit: 100 } });
  expect(traces).toMatchObject({ id: "traces", query: { expression: '{ resource.service.name = "api" }', limit: 30 } });
  expect(logs.query.end - logs.query.start).toBeLessThanOrEqual(86400);
  expect(traces.query.end - traces.query.start).toBeLessThanOrEqual(86400);
  const logCard = page.locator(".dashboard-widget").filter({ hasText: "인증 로그" });
  await expect(logCard).toContainText("<img src=x onerror=alert(1)> raw log");
  await expect(logCard.locator("img")).toHaveCount(0);
  await expect(page.locator(".dashboard-widget").filter({ hasText: "인증 트레이스" })).toContainText("GET /health");
});

test("edits layout by keyboard and pointer, then restores it after reload", async ({ page }) => {
  await page.locator("#dashboard-edit").click();
  const widget = page.locator(".dashboard-widget").first();
  const id = await widget.getAttribute("data-widget-id");
  const before = await widget.boundingBox();
  let initialWidth = 0;
  if (test.info().project.name !== "compact") {
    await widget.locator('[data-widget-action="settings"]').click();
    initialWidth = Number(await page.locator('#dashboard-widget-dialog [name="w"]').inputValue());
    await page.locator('#dashboard-widget-dialog [data-dashboard-close]').first().click();
  }
  if (test.info().project.name === "compact") {
    await widget.locator('[data-widget-action="settings"]').click();
    await page.locator('#dashboard-widget-dialog [name="w"]').fill("7");
    await page.locator('#dashboard-widget-dialog [name="h"]').fill("4");
    await page.locator("#dashboard-widget-form").getByRole("button", { name: "저장", exact: true }).click();
  } else {
    await widget.locator('[data-widget-action="move"]').click();
    await page.keyboard.press("ArrowRight");
    await widget.locator('[data-widget-action="resize"]').click();
    await page.keyboard.press("ArrowRight");
    const title = widget.locator(".dashboard-widget-title");
    const box = await title.boundingBox();
    if (box) {
      await page.mouse.move(box.x + 8, box.y + 8);
      await page.mouse.down();
      await page.mouse.move(box.x + 80, box.y + 8);
      await page.mouse.up();
    }
  }
  await page.locator("#dashboard-edit").click();
  const saved = await page.evaluate((key) => JSON.parse(localStorage.getItem(key) || "{}"), layoutKey);
  const persisted = saved.widgets.find((w: any) => w.id === id);
  expect(persisted).toBeTruthy();
  if (test.info().project.name === "compact") {
    expect(persisted.w).toBe(7);
    expect(persisted.h).toBe(4);
  } else {
    expect(persisted.x).toBeGreaterThan(0);
    expect(persisted.w).toBeGreaterThan(initialWidth);
  }
  await page.reload();
  const restored = page.locator(`.dashboard-widget[data-widget-id="${id}"]`);
  await expect(restored).toBeVisible();
  if (test.info().project.name !== "compact") expect(await restored.boundingBox()).not.toEqual(before);
});

test("persists an explicitly empty board", async ({ page }) => {
  await page.locator("#dashboard-edit").click();
  while (await page.locator(".dashboard-widget").count()) {
    await page.locator('.dashboard-widget [data-widget-action="remove"]').first().click();
  }
  await page.locator("#dashboard-edit").click();
  await page.reload();
  await expect(page.locator(".dashboard-widget")).toHaveCount(0);
  const saved = await page.evaluate((key) => JSON.parse(localStorage.getItem(key) || "{}"), layoutKey);
  expect(saved.widgets).toEqual([]);
});

test("keeps the draft on a failed save and permits a retry", async ({ page }) => {
  await page.locator("#dashboard-edit").click();
  await addMetric(page, "저장 재시도", "up");
  await page.evaluate(() => { (window as any).dashboardFixture.failSave = true; });
  await page.locator("#dashboard-edit").click();
  await expect(page.locator("#dashboard-edit")).toHaveText("저장 · 배치 완료");
  await expect(page.locator(".dashboard-widget").filter({ hasText: "저장 재시도" })).toContainText("저장 재시도");
  await page.evaluate(() => { (window as any).dashboardFixture.failSave = false; });
  await page.locator("#dashboard-edit").click();
  await expect(page.locator("#dashboard-edit")).toHaveText("배치 편집");
});

test("preserves unreadable or newer saved layouts without overwriting them", async ({ page }) => {
  const raw = '{"version":99,"widgets":["do not migrate"]}';
  await page.evaluate(([key, value]) => localStorage.setItem(key, value), [layoutKey, raw]);
  await page.reload();
  await expect(page.locator("#dashboard-edit")).toBeDisabled();
  await expect.poll(() => page.evaluate((key) => localStorage.getItem(key), layoutKey)).toBe(raw);
});

test("polling preserves focused widget settings and a missing source stays missing", async ({ page }) => {
  await page.locator("#dashboard-edit").click();
  await addMetric(page, "사라진 지표", "up");
  const widget = page.locator(".dashboard-widget").filter({ hasText: "사라진 지표" });
  const id = await widget.getAttribute("data-widget-id");
  await widget.locator('[data-widget-action="settings"]').click();
  const expression = page.locator('#dashboard-widget-dialog [name="expression"]');
  await expression.focus();
  await page.waitForTimeout(2200);
  await expect(expression).toBeFocused();
  await page.locator('#dashboard-widget-dialog [data-dashboard-close]').first().click();
  await page.locator("#dashboard-edit").click();
  await page.reload();
  await page.evaluate(() => { (window as any).dashboardFixture.removeMetrics = true; });
  await page.waitForTimeout(2200);
  const missing = page.locator(`.dashboard-widget[data-widget-id="${id}"]`);
  await expect(missing.locator(".dashboard-widget-body")).toContainText(/연결.*없|missing/i);
});

test("does not overflow a narrow viewport", async ({ page }) => {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
});

test("Ctrl wheel zooms only time, preserving value scale and node size", async ({ page }) => {
  await page.evaluate(() => {
    (window as any).go.main.App.Query = async () => {
      (window as any).dashboardFixture.queryCalls++;
      const end = Math.floor(Date.now() / 1000);
      return { type: "matrix", warnings: [], truncated: false, series: [{ labels: {}, points: [
        { time: end - 600, value: 1 }, { time: end - 300, value: 5 }, { time: end, value: 2 },
      ] }] };
    };
  });
  await page.locator("#dashboard-edit").click();
  await addMetric(page, "확대 대상", "up");
  await addMetric(page, "다른 노드", "up");
  await page.locator("#dashboard-edit").click();
  const card = page.locator(".dashboard-widget").filter({ hasText: "확대 대상" });
  const other = page.locator(".dashboard-widget").filter({ hasText: "다른 노드" });
  await card.scrollIntoViewIfNeeded();
  const size = await card.boundingBox();
  const graphSize = await card.locator(".db-chart").boundingBox();
  const coordinates = async () => card.locator(".db-chart path").evaluate(el =>
    [...(el.getAttribute("d") || "").matchAll(/[ML](-?[\d.]+),(-?[\d.]+)/g)].map(match => ({ x: Number(match[1]), y: Number(match[2]) })));
  const original = await coordinates();
  await expect(card.locator(".db-y-axis span")).toHaveText(["5", "3", "1"]);
  await expect(card.locator(".db-end-values .db-current")).toHaveText("2");
  expect(await card.locator(".db-end-values .db-current").evaluate(el => getComputedStyle(el).fontSize)).toBe(await card.locator(".db-y-axis span").first().evaluate(el => getComputedStyle(el).fontSize));
  await expect(card).not.toContainText("최근 값");
  const marker = await card.locator(".db-end-values .db-current").boundingBox();
  expect(marker!.x).toBeGreaterThanOrEqual(graphSize!.x + graphSize!.width);
  expect(Math.abs(marker!.y + marker!.height / 2 - (graphSize!.y + graphSize!.height * .75))).toBeLessThan(2);
  await card.locator(".db-chart").hover({ position: { x: graphSize!.width / 2, y: graphSize!.height / 2 } });
  await expect(card.getByRole("tooltip")).toContainText("5");
  await expect(card.getByRole("tooltip")).toContainText("value");
  await page.mouse.move(0, 0);
  await expect(card.getByRole("tooltip")).toBeHidden();
  const valueRange = await card.locator(".db-chart-scale > span").textContent();
  const timeRange = await card.locator(".db-chart-axis").textContent();
  const calls = await page.evaluate(() => (window as any).dashboardFixture.queryCalls);
  const wheel = async (ctrlKey: boolean, deltaY: number) => card.locator(".db-chart").evaluate((el, args) => {
    const bounds = el.getBoundingClientRect();
    const event = new WheelEvent("wheel", { ...args, bubbles: true, cancelable: true, clientX: bounds.x + bounds.width / 2, clientY: bounds.y + bounds.height / 4 });
    el.dispatchEvent(event);
    return event.defaultPrevented;
  }, { ctrlKey, deltaY });
  expect(await wheel(false, -200)).toBe(false);
  await expect(card.locator('[data-graph-action="reset"]')).toHaveText("100%");
  expect(await wheel(true, -200)).toBe(true);
  await expect(card.locator('[data-graph-action="reset"]')).toHaveText("141%");
  await expect(other.locator('[data-graph-action="reset"]')).toHaveText("100%");
  expect(await card.boundingBox()).toEqual(size);
  expect(await card.locator(".db-chart").boundingBox()).toEqual(graphSize);
  const zoomed = await coordinates();
  expect(zoomed.map(point => point.y)).toEqual(original.map(point => point.y));
  expect(zoomed[2].x - zoomed[0].x).toBeGreaterThan(original[2].x - original[0].x);
  expect(Math.abs(zoomed[1].x - original[1].x)).toBeLessThan(0.5);
  expect(await card.locator(".db-chart-scale > span").textContent()).toBe(valueRange);
  expect(await card.locator(".db-chart-axis").textContent()).not.toBe(timeRange);
  for (let i = 0; i < 10; i++) await wheel(true, -200);
  await expect(card.locator('[data-graph-action="reset"]')).toHaveText("800%");
  await card.getByRole("button", { name: "전체 시간 범위" }).click();
  await expect(card.locator('[data-graph-action="reset"]')).toHaveText("100%");
  expect(await coordinates()).toEqual(original);
  await card.getByRole("button", { name: "시간축 확대", exact: true }).click();
  await expect(card.locator('[data-graph-action="reset"]')).toHaveText("125%");
  await expect(card.getByRole("button", { name: "시간축 확대", exact: true })).toBeFocused();
  for (let i = 0; i < 10; i++) await wheel(true, 200);
  await expect(card.locator('[data-graph-action="reset"]')).toHaveText("100%");
  expect(await page.evaluate(() => (window as any).dashboardFixture.queryCalls)).toBe(calls);
});
