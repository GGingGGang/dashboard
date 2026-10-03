import { test, expect } from "@playwright/test";

test("a registered combined platform uses its own query language and keeps targets", async ({
  page,
}) => {
  await page.addInitScript(() => {
    const now = new Date().toISOString();
    const rule = {
      id: "usage",
      name: "Custom usage",
      expression: "SELECT usage",
      threshold: 90,
      unit: "%",
      description: "Custom language",
      enabled: true,
    };
    const connection = {
      id: "custom",
      kind: "external-platform",
      name: "External platform",
      url: "https://example.invalid",
      browserUrl: "",
      auth: "none",
      username: "",
      caFile: "",
      targets: [
        { id: "pipeline", name: "Pipeline", service: "", environment: "" },
      ],
      rules: [rule],
    };
    const provider = {
      kind: "external-platform",
      name: "External platform",
      category: "combined",
      capabilities: [
        "builds",
        "deployments",
        "metrics",
        "instant",
        "range",
        "rules",
      ],
      defaultAuth: "none",
      authMethods: ["none"],
      pollSeconds: 15,
      query: {
        language: "SQL",
        defaultExpression: "SELECT usage",
        presets: [rule],
      },
    };
    Object.defineProperty(window, "go", {
      value: {
        main: {
          App: {
            GetPreference: async () => "",
            SavePreference: async () => {},
            Refresh: async () => {},
            GetState: async () => ({
              connections: [connection],
              providers: [provider],
              snapshots: [
                {
                  connectionId: "custom",
                  attempted: now,
                  lastSuccess: now,
                  error: "",
                  storageError: "",
                  builds: [],
                  queue: [],
                  deployments: [],
                  rules: [],
                },
              ],
              demo: true,
              error: "",
            }),
            Presets: async () => [rule],
            Query: async (_id: string, q: { expression: string }) => {
              if (q.expression !== "SELECT usage")
                throw new Error("wrong language");
              return {
                type: "vector",
                series: [
                  {
                    labels: { node: "one" },
                    points: [{ time: Date.now() / 1000, value: 95 }],
                  },
                ],
                warnings: [],
                truncated: false,
              };
            },
          },
        },
      },
    });
  });
  await page.goto("/");
  await page.getByRole("button", { name: "지표 탐색", exact: true }).click();
  await expect(page.locator("#query-expression")).toHaveValue("SELECT usage");
  await expect(
    page.getByText("SQL 쿼리를 직접 실행합니다.", { exact: false }),
  ).toBeVisible();
  await page.getByRole("button", { name: "쿼리 실행", exact: true }).click();
  await expect(page.locator("#query-result tbody")).toContainText("95");
  await page.getByRole("button", { name: "연결 설정", exact: true }).click();
  await expect(page.locator(".connection-card")).toContainText(
    "1개 감시 대상 · 1개 규칙",
  );
  await page.getByRole("button", { name: "설정 편집" }).click();
  await expect(page.locator(".target-card")).toHaveCount(1);
  await expect(page.locator("#rules")).toContainText("Custom usage");
  await expect(page.locator("#rule-expression-0")).toHaveValue("SELECT usage");
});

test("Argo CD distinguishes session login from API token authentication", async ({
  page,
}) => {
  await page.goto("/?empty");
  await page.getByRole("button", { name: "연결 추가", exact: true }).click();
  await page.locator("[name=kind]").selectOption("argocd");
  await expect(page.locator("[name=auth]")).toHaveValue("bearer");
  await expect(page.locator('[name=auth] option[value="basic"]')).toHaveCount(
    0,
  );
  await expect(page.locator("[name=username]")).toBeHidden();
  await expect(page.locator("#auth-help")).toContainText(
    "계정 비밀번호는 입력하지 않습니다",
  );
  await page.locator("[name=secret]").fill("example-token");
  await page.locator("[name=auth]").selectOption("argocd-login");
  await expect(page.locator("[name=username]")).toBeVisible();
  await expect(page.locator("#secret-label")).toHaveText("Argo CD 비밀번호");
  await expect(page.locator("[name=secret]")).toHaveValue("");
  await page.locator("[name=name]").fill("My Argo CD");
  await page.locator("[name=url]").fill("https://deploy.example.org");
  await page.locator("[name=username]").fill("reader");
  await page.locator("[name=secret]").fill("example-password");
  await page.getByRole("button", { name: "저장", exact: true }).click();
  await expect(page.locator(".connection-card")).toContainText("argocd-login");
});

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
  const payment = page.getByRole("checkbox", { name: "payments 감시" });
  await page.locator(".target-card").filter({ hasText: "payments" }).click();
  await expect(payment).toBeChecked();
  await expect(
    page.getByRole("checkbox", { name: "catalog 감시" }),
  ).not.toBeChecked();
  await expect(page.locator("#target-count")).toContainText("1개 선택");
  await payment.focus();
  await payment.press("Space");
  await expect(payment).not.toBeChecked();
  await payment.press("Space");
  await expect(payment).toBeChecked();
  const cards = await page.locator(".target-card").evaluateAll((els) =>
    els.map((el) => ({
      top: el.getBoundingClientRect().top,
      height: el.getBoundingClientRect().height,
    })),
  );
  expect(cards[0].top).toBe(cards[1].top);
  expect(cards[0].height).toBeLessThan(90);
  await page.locator("#targets").screenshot({
    path: `test-results/target-cards-${test.info().project.name}.png`,
  });
  await page.locator(".target-mapping summary").click();
  await page
    .getByRole("textbox", { name: "payments 서비스", exact: true })
    .fill("checkout");
  await expect(payment).toBeChecked();
  await page.getByRole("button", { name: "저장", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "My CI", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".connection-card")).toContainText("1개 감시 대상");
  await page.getByRole("button", { name: "설정 편집" }).click();
  await expect(
    page.getByRole("checkbox", { name: "payments 감시" }),
  ).toBeChecked();
  await page.locator(".target-mapping summary").click();
  await expect(
    page.getByRole("textbox", { name: "payments 서비스", exact: true }),
  ).toHaveValue("checkout");
});
