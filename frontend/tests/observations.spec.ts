import { test, expect } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const w = window as any;
    const connection = (id: string, kind: string) => ({ id, kind, name: id, url: "https://example.invalid", auth: "none", browserUrl: "", username: "", caFile: "", targets: [], rules: [], revision: "1" });
    const observation = (id: string) => ({ connectionId: id, revision: "1", requestId: "test", observedAt: new Date().toISOString(), windowStart: 1700000000, windowEnd: 1700000001, outcome: "partial", completeness: "unknown", diagnostics: ["limit reached"] });
    const logs = (id: string) => ({ observation: observation(id), entries: [{ timestampNs: "1700000000123456789", line: `<img src=x onerror=alert(1)> ${id}`, labels: { service: "api" }, fields: { trace_id: "0123456789abcdef0123456789abcdef" } }] });
    w.go = { main: { App: {
      GetPreference: async () => "", SavePreference: async () => {}, Refresh: async () => {},
      GetState: async () => ({ connections: [connection("first", "loki"), connection("second", "loki"), connection("tempo", "tempo")], snapshots: [], providers: [
        { kind: "loki", name: "Loki", onDemand: true, capabilities: ["logs"], queries: { logs: { language: "LogQL", defaultExpression: '{job="api"}', maxResults: 500 } } },
        { kind: "tempo", name: "Tempo", onDemand: true, capabilities: ["traces"], queries: { traces: { language: "TraceQL", defaultExpression: '{ true }', maxResults: 100 } } },
      ], demo: true, error: "" }),
      SearchLogs: async (id: string) => {
        if (w.delay && id === "first") return new Promise((resolve, reject) => { w.finishOld = (fail: boolean) => fail ? reject(new Error("obsolete-error")) : resolve(logs("obsolete-result")); });
        return logs(id);
      },
      SearchTraces: async () => ({ observation: observation("tempo"), traces: [{ traceId: "0123456789abcdef0123456789abcdef", rootService: "api", rootName: "GET /", startNs: "1700000000123456789", durationMs: 1 }] }),
      Trace: async (_id: string, traceId: string) => ({ observation: observation("tempo"), traceId, spans: [{ traceId, spanId: "0102030405060708", parentSpanId: "0807060504030201", name: "GET /", service: "api", startNs: "1700000000123456789", durationNs: "1", attributes: [{ key: "large", value: { intValue: "9223372036854775807" } }], resource: [], details: {}, scope: {} }] }),
    } } };
  });
  await page.goto("/");
});

test("logs retain ns and treat payload as text", async ({ page }) => {
  await expect(page.locator("#page")).not.toContainText("수집 대기 또는 조회 지연");
  await page.getByRole("button", { name: "로그 탐색", exact: true }).click();
  await page.getByRole("button", { name: "검색", exact: true }).click();
  await expect(page.locator("#explore-result")).toContainText("1700000000123456789 ns");
  await expect(page.locator("#explore-result")).toContainText("<img src=x onerror=alert(1)>");
  await expect(page.locator("#explore-result img")).toHaveCount(0);
  await expect(page.locator("#explore-result")).toContainText("전체 데이터 여부 미확인");
  await page.getByText("라벨 · 구조화 필드", { exact: true }).click();
  await expect(page.locator("#explore-result")).toContainText("0123456789abcdef0123456789abcdef");
});

for (const fail of [false, true]) test(`late log ${fail ? "failure" : "success"} is discarded`, async ({ page }) => {
  await page.evaluate(() => { (window as any).delay = true; });
  await page.getByRole("button", { name: "로그 탐색", exact: true }).click();
  await page.getByRole("button", { name: "검색", exact: true }).click();
  await expect.poll(() => page.evaluate(() => typeof (window as any).finishOld)).toBe("function");
  await page.locator("#explore-connection").selectOption("second");
  await page.getByRole("button", { name: "검색", exact: true }).click();
  await expect(page.locator("#explore-result")).toContainText("second");
  await page.evaluate(async fail => { (window as any).finishOld(fail); await new Promise(resolve => setTimeout(resolve, 0)); }, fail);
  await expect(page.locator("#explore-result")).not.toContainText("obsolete");
});

test("trace search opens detail without inventing a root", async ({ page }) => {
  await page.getByRole("button", { name: "트레이스 탐색", exact: true }).click();
  await page.getByRole("button", { name: "검색", exact: true }).click();
  await page.getByRole("button", { name: "0123456789abcdef0123456789abcdef", exact: true }).click();
  await page.locator(".span-detail summary").click();
  await expect(page.locator("#trace-result")).toContainText("응답에 부모 span 없음");
  await expect(page.locator("#trace-result")).toContainText("9223372036854775807");
  await expect(page.locator("#trace-result")).toContainText("1700000000123456789 ns");
  await page.locator("#trace-id").fill("0123456789abcdef0123456789abcdef");
  await page.getByRole("button", { name: "ID로 상세 조회", exact: true }).click();
  await expect(page.locator("#trace-result")).toContainText("트레이스 상세");
});
