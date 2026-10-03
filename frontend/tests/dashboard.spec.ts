import { test, expect } from "@playwright/test";

test("failed metric rules never produce a healthy zero summary", async ({
  page,
}) => {
  await page.addInitScript(() => {
    const timestamp = new Date().toISOString();
    Object.defineProperty(window, "go", {
      value: {
        main: {
          App: {
            GetPreference: async () => "",
            SavePreference: async () => {},
            Refresh: async () => {},
            Query: async () => {
              throw new Error("upstream unavailable");
            },
            GetState: async () => ({
              demo: true,
              error: "",
              providers: [
                { kind: "prometheus", capabilities: ["promql", "rules"] },
              ],
              connections: [
                {
                  id: "metrics",
                  kind: "prometheus",
                  name: "Metrics",
                  targets: [],
                  rules: [],
                },
              ],
              snapshots: [
                {
                  connectionId: "metrics",
                  attempted: timestamp,
                  lastSuccess: timestamp,
                  error: "",
                  storageError: "",
                  builds: [],
                  queue: [],
                  deployments: [],
                  rules: [
                    {
                      rule: {
                        id: "up",
                        name: "Targets",
                        expression: "up",
                        threshold: 0,
                      },
                      result: { series: [], warnings: [] },
                      breaches: 0,
                      error: "query rejected",
                    },
                  ],
                },
              ],
            }),
          },
        },
      },
    });
  });
  await page.goto("/");
  const summary = page.locator(".stat").filter({ hasText: "지표 이상 징후" });
  await expect(summary.locator(".value")).toHaveText("—");
  await expect(summary).toContainText("판정 가능 0/1개 규칙");
  await expect(page.getByText("판정 불가", { exact: true })).toBeVisible();
});

test("overview preserves separate sync and health and fits both sizes", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "운영 현황", exact: true }),
  ).toBeVisible();
  await expect(
    page.locator(".services").getByText("OutOfSync", { exact: true }),
  ).toBeVisible();
  await expect(
    page.locator(".services").getByText("Healthy", { exact: true }).first(),
  ).toBeVisible();
  await page.locator(".services summary").first().click();
  await expect(page.getByText("배포 revision:")).toHaveCount(2);
  const beforePoll = await page.locator("#page > .footer").innerText();
  await page.locator("#scope").focus();
  await page.waitForTimeout(2300);
  await expect(page.locator("#page > .footer")).not.toHaveText(beforePoll);
  await expect(page.locator("#scope")).toBeFocused();
  await expect(page.locator(".services details").first()).toHaveAttribute(
    "open",
    "",
  );
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBeTruthy();
  expect(errors).toEqual([]);
  await page.screenshot({
    path: `test-results/overview-${test.info().project.name}.png`,
    fullPage: true,
  });
});

test("archive filters failure and remains available after removing connection", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByRole("button", { name: "빌드 이력", exact: true }).click();
  await page.locator("[name=status]").selectOption("FAILURE");
  await page.getByRole("button", { name: "조회", exact: true }).click();
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await page.getByRole("button", { name: "연결 설정", exact: true }).click();
  page.on("dialog", (d) => d.accept());
  await page
    .locator(".connection-card")
    .filter({ hasText: "Demo · Jenkins" })
    .getByRole("button", { name: "연결 삭제" })
    .click();
  await expect(page.locator(".connection-card")).toHaveCount(2);
  await page.getByRole("button", { name: "빌드 이력", exact: true }).click();
  await expect(page.locator("tbody tr")).toHaveCount(24);
});

test("query executes, saves favorite, and reports rejected expressions", async ({
  page,
}) => {
  await page.goto("/");
  await page.getByRole("button", { name: "지표 탐색", exact: true }).click();
  await page.getByRole("button", { name: "쿼리 실행", exact: true }).click();
  await expect(page.getByRole("heading", { name: "조회 결과" })).toBeVisible();
  await page.getByRole("button", { name: "즐겨찾기 저장" }).click();
  await expect(page.locator("#favorite option")).toHaveCount(2);
  await page.locator("#query-expression").fill("invalid(");
  await page.getByRole("button", { name: "쿼리 실행" }).click();
  await expect(page.locator("#query-result")).toContainText("rejected");
});

test("empty onboarding discovers and selects a target", async ({ page }) => {
  await page.goto("/?empty");
  await page.getByRole("button", { name: "연결 추가", exact: true }).click();
  await page.locator("[name=name]").fill("My CI");
  await page.locator("[name=url]").fill("https://ci.example.org/jenkins");
  await page.locator("[name=auth]").selectOption("none");
  await page.getByRole("button", { name: "연결 검사 · 대상 찾기" }).click();
  await expect(page.locator("#test-result")).toContainText("연결 성공");
  await page.getByRole("checkbox", { name: "payments 감시" }).check();
  await page.getByRole("button", { name: "저장", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "My CI", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".connection-card")).toContainText("1개 감시 대상");
});
