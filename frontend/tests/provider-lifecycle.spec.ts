import { test, expect } from "@playwright/test";

for (const outcome of ["success", "failure"]) {
  test(`late query ${outcome} cannot replace the selected connection`, async ({ page }) => {
    await page.addInitScript(() => {
      const w = window as any;
      const connection = (id: string) => ({ id, kind: "fixture", name: id, url: "https://example.invalid", auth: "none", browserUrl: "", username: "", caFile: "", targets: [], rules: [], revision: "1" });
      const result = (id: string) => ({ type: "vector", warnings: [], truncated: false, series: [{ labels: { source: id }, points: [{ time: Date.now() / 1000, value: 1 }] }] });
      w.go = { main: { App: {
        GetPreference: async () => "", SavePreference: async () => {}, Refresh: async () => {},
        GetState: async () => ({ connections: [connection("first"), connection("second")], snapshots: [], providers: [{ kind: "fixture", name: "Fixture", capabilities: ["metrics"], defaultAuth: "none", authMethods: ["none"], query: { language: "Fixture", defaultExpression: "value", presets: [] } }], demo: true, error: "" }),
        Query: async (id: string) => {
          if (id === "second") return result("current-result");
          return new Promise((resolve, reject) => {
            w.finishOld = (fail: boolean) => fail ? reject(new Error("obsolete-error")) : resolve(result("obsolete-result"));
          });
        },
      } } };
    });
    await page.goto("/");
    await page.getByRole("button", { name: "지표 탐색", exact: true }).click();
    await page.getByRole("button", { name: "쿼리 실행", exact: true }).click();
    await expect.poll(() => page.evaluate(() => typeof (window as any).finishOld)).toBe("function");
    await page.locator("#query-connection").selectOption("second");
    await page.getByRole("button", { name: "쿼리 실행", exact: true }).click();
    await expect(page.locator("#query-result")).toContainText("current-result");
    await page.evaluate(async (fail) => { (window as any).finishOld(fail); await new Promise((resolve) => setTimeout(resolve, 0)); }, outcome === "failure");
    await expect(page.locator("#query-result")).toContainText("current-result");
    await expect(page.locator("#query-result")).not.toContainText("obsolete");
  });
}

test("denied discovery keeps selected targets and allows a direct target", async ({ page }) => {
  await page.addInitScript(() => {
    const w = window as any;
    const connection = { id: "c", kind: "fixture", name: "Direct targets", url: "https://example.invalid", auth: "none", browserUrl: "", username: "", caFile: "", targets: [{ id: "existing", name: "Existing", service: "", environment: "" }], rules: [] };
    w.go = { main: { App: {
      GetPreference: async () => "", SavePreference: async () => {}, Refresh: async () => {}, Presets: async () => [],
      GetState: async () => ({ connections: [connection], snapshots: [], providers: [{ kind: "fixture", name: "Fixture", capabilities: ["builds"], defaultAuth: "none", authMethods: ["none"] }], demo: true, error: "" }),
      TestConnection: async () => ({ targets: [], discovery: "failed", warning: "listing denied" }),
      SaveConnection: async (input: any) => { w.savedTargets = input.connection.targets; return "c"; },
    } } };
  });
  await page.goto("/");
  await page.getByRole("button", { name: "연결 설정", exact: true }).click();
  await page.getByRole("button", { name: "설정 편집", exact: true }).click();
  await page.getByRole("button", { name: "연결 검사 · 대상 찾기", exact: true }).click();
  await expect(page.locator("#test-result")).toContainText("listing denied");
  await expect(page.getByRole("checkbox", { name: "Existing 감시", exact: true })).toBeChecked();
  await page.locator("#manual-target").fill("direct/pipeline");
  await page.getByRole("button", { name: "대상 추가", exact: true }).click();
  await expect(page.getByRole("checkbox", { name: "direct/pipeline 감시", exact: true })).toBeChecked();
  await page.getByRole("button", { name: "저장", exact: true }).click();
  expect(await page.evaluate(() => (window as any).savedTargets.map((t: any) => t.id))).toEqual(["existing", "direct/pipeline"]);
});
