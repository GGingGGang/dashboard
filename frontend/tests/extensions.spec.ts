import { test, expect } from "@playwright/test";

test("plugin lifecycle preserves connections and gates registered sources", async ({ page }) => {
  await page.goto("/");
  await page.locator('nav [data-page="dashboard"]').click();
  const result = await page.evaluate(async () => {
    // @ts-expect-error Vite resolves the browser-side module path.
    const { previewBridge } = await import("/src/bridge/preview.ts");
    const api = previewBridge();
    const before = await api.GetState();
    const connection = before.connections.find((c: { kind: string }) => c.kind === "prometheus")!;
    const sources = await api.Sources(connection.id);
    const query = { expression: "up", start: 0, end: 0 };
    const value = await api.QuerySource(connection.id, "default", query);
    await api.SetPluginEnabled("prometheus", false);
    let rejected = false;
    try { await api.QuerySource(connection.id, "default", query); } catch { rejected = true; }
    const disabled = await api.Plugins();
    const retained = await api.GetState();
    const preference = await api.GetPreference("plugins.disabled");
    await api.SetPluginEnabled("prometheus", true);
    const enabled = await api.Plugins();
    const restored = await api.QuerySource(connection.id, "default", query);
    return { sourceCount: sources.length, value: value.series[0].points[0].value,
      rejected, disabled: disabled.find((p: { manifest: { id: string } }) => p.manifest.id === "prometheus")!.enabled,
      retained: retained.connections.length === before.connections.length, preference,
      enabled: enabled.find((p: { manifest: { id: string } }) => p.manifest.id === "prometheus")!.enabled,
      restored: restored.series[0].points[0].value };
  });
  expect(result).toEqual({ sourceCount: 1, value: 1, rejected: true, disabled: false,
    retained: true, preference: '["prometheus"]', enabled: true, restored: 1 });
});
