import { test, expect } from "@playwright/test";

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const w = window as any;
    w.reads = [];
    w.go = { main: { App: {
      GetState: async () => ({ connections: [], snapshots: [], providers: [], demo: false, error: "" }),
      GetPreference: async (key: string) => localStorage.getItem(key) || "",
      SavePreference: async (key: string, value: string) => localStorage.setItem(key, value),
      Refresh: async () => {},
      ReadJSON: async (request: any) => {
        w.reads.push(request);
        if (request.filter === "invalid") throw new Error("jq invalid");
        const text = request.filter === ".text" ? "<img src=x onerror=alert(1)>" : "123123";
        return { text, json: request.filter === ".text" ? JSON.stringify(text) : text, scalar: true, count: 1, observedAt: new Date().toISOString() };
      },
    } } };
  });
  await page.goto("/");
  await page.getByRole("button", { name: "API 쿼리", exact: true }).click();
});

test("HTTP jq result becomes a saved manual node without duplicate requests", async ({ page }) => {
  await expect(page.locator('[name="source"] option[value="command"]')).toHaveAttribute("disabled", "");
  await page.getByLabel("API URL · GET").fill("http://localhost:8080/status");
  await page.getByLabel("노드 이름").fill("서비스 이름");
  await page.getByRole("button", { name: "조회 실행" }).click();
  await expect(page.locator(".api-result .db-current")).toHaveText("123123");
  await page.getByRole("button", { name: "대시보드에 추가" }).click();
  const node = page.locator(".dashboard-widget").filter({ hasText: "서비스 이름" });
  await expect(node.locator(".db-current")).toHaveText("123123");
  await page.getByRole("button", { name: "저장 · 배치 완료" }).click();
  expect(await page.evaluate(() => (window as any).reads)).toEqual([{ source: "http", url: "http://localhost:8080/status", filter: ".service.file.name" }]);
  await page.locator("#refresh").click();
  expect(await page.evaluate(() => (window as any).reads.length)).toBe(1);
  await page.reload();
  await expect(node).toContainText("아직 조회된 데이터");
  expect(await page.evaluate(() => (window as any).reads.length)).toBe(0);
  await node.getByRole("button", { name: "이 정보 새로고침" }).click();
  await expect(node.locator(".db-current")).toHaveText("123123");
  expect(await page.evaluate(() => (window as any).reads.length)).toBe(1);
  await page.getByRole("button", { name: "배치 편집" }).click();
  await node.getByRole("button", { name: "정보 설정" }).click();
  await expect(page.getByLabel("jq 조회식")).toHaveValue(".service.file.name");
  await page.getByLabel("jq 조회식").fill(".text");
  await page.getByRole("button", { name: "조회 실행" }).click();
  await expect(page.locator(".api-result .db-current")).toHaveText("<img src=x onerror=alert(1)>");
  await expect(page.locator(".api-result img")).toHaveCount(0);
  await page.getByRole("button", { name: "노드 설정 적용" }).click();
  await expect(page.locator(".dashboard-widget")).toHaveCount(1);
  await expect(node.locator(".db-current")).toHaveText("<img src=x onerror=alert(1)>");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test("invalid and stale queries cannot be added or overwrite newer settings", async ({ page }) => {
  await page.getByLabel("API URL · GET").fill("http://localhost:8080/status");
  await page.getByLabel("jq 조회식").fill("invalid");
  await page.getByRole("button", { name: "조회 실행" }).click();
  await expect(page.getByRole("alert")).toContainText("jq invalid");
  await expect(page.getByRole("button", { name: "대시보드에 추가" })).toBeDisabled();
  await page.evaluate(() => { (window as any).go.main.App.ReadJSON = () => new Promise(resolve => { (window as any).reply = resolve; }); });
  await page.getByLabel("jq 조회식").fill(".");
  await page.getByRole("button", { name: "조회 실행" }).click();
  await page.getByLabel("jq 조회식").fill(".new");
  await page.evaluate(() => (window as any).reply({ text: "OLD", json: '"OLD"', scalar: true, count: 1, observedAt: new Date().toISOString() }));
  await expect(page.getByRole("button", { name: "대시보드에 추가" })).toBeDisabled();
  await expect(page.locator(".api-result")).not.toContainText("OLD");
});

test("saved API nodes reject credentials, unknown sources and automatic refresh", async ({ page }) => {
  const results = await page.evaluate(async () => {
    const { parseLayout } = await import("/src/components/dashboard-layout.ts");
    const widget = { id: "json", kind: "json", title: "JSON", connectionId: "", targetId: "", expression: "", unit: "", period: 900, refreshSeconds: 0, x: 0, y: 0, w: 4, h: 3, json: { source: "http", url: "https://example.invalid", filter: "." } };
    return [widget, { ...widget, refreshSeconds: 30 }, { ...widget, json: { ...widget.json, source: "command" } }, { ...widget, json: { ...widget.json, url: "https://user:secret@example.invalid" } }].map(value => {
      try { parseLayout(JSON.stringify({ version: 1, widgets: [value] })); return true; } catch { return false; }
    });
  });
  expect(results).toEqual([true, false, false, false]);
});
