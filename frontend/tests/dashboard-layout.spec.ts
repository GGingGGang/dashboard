import { expect, test } from "@playwright/test";
import { defaultLayout, parseLayout, placeWidget, type DashboardWidget } from "../src/components/dashboard-layout";
import type { State } from "../src/shared/types";

const widget = (id: string, x = 0, y = 0): DashboardWidget => ({ id, kind: "builds", title: id, connectionId: "ci", targetId: "job", expression: "", unit: "", period: 3600, refreshSeconds: 30, x, y, w: 3, h: 2 });

test("layout rejects malformed, future, and duplicate saved widgets", () => {
  expect(parseLayout("")).toBeNull();
  expect(() => parseLayout("not json")).toThrow(/JSON/);
  expect(() => parseLayout('{"version":2,"widgets":[]}')).toThrow(/지원하지 않는/);
  expect(() => parseLayout(JSON.stringify({ version: 1, widgets: [widget("same"), widget("same", 3)] }))).toThrow(/중복/);
  expect(() => parseLayout(JSON.stringify({ version: 1, widgets: [widget("one"), widget("two", 1)] }))).toThrow(/겹/);
});

test("moving a widget pushes only its collision chain downward", () => {
  const result = placeWidget([widget("a", 0, 6), widget("b", 0, 1), widget("c", 0, 3), widget("gap", 6, 0)], widget("a", 0, 0));
  expect(result.find(item => item.id === "a")?.y).toBe(0);
  expect(result.find(item => item.id === "b")?.y).toBe(2);
  expect(result.find(item => item.id === "c")?.y).toBe(4);
  expect(result.find(item => item.id === "gap")?.y).toBe(0);
  for (const left of result) for (const right of result) if (left.id < right.id) expect(left.x < right.x + right.w && left.x + left.w > right.x && left.y < right.y + right.h && left.y + left.h > right.y).toBeFalsy();
});

test("collision resolution is independent of the stored widget order", () => {
  const result = placeWidget([widget("low", 0, 3), widget("high", 0, 1)], widget("changed", 0, 0));
  expect(result.find(item => item.id === "high")?.y).toBe(2);
  expect(result.find(item => item.id === "low")?.y).toBe(4);
});

test("default cards use supported providers and target capabilities", () => {
  const targetID = "folder/".repeat(300) + "build";
  const targetName = "운영 파이프라인 ".repeat(20);
  const ruleName = "응답 시간 ".repeat(20);
  const unit = "milliseconds ".repeat(5);
  const state: State = {
    error: "", demo: false, snapshots: [{ connectionId: "ci", attempted: "", lastSuccess: "", error: "", storageError: "", builds: [], queue: [{ id: 1, job: targetID, since: 0, reason: "", url: "" }], deployments: [], rules: [], imported: 0, backfillPending: false }],
    providers: [
      { kind: "ci", name: "CI", category: "ci", capabilities: ["builds", "queue"], defaultAuth: "none", authMethods: [] },
      { kind: "observability", name: "Observe", category: "observability", capabilities: ["metrics", "logs"], defaultAuth: "none", authMethods: [], queries: { logs: { language: "LogQL", defaultExpression: "{app=\"api\"}", maxResults: 10 } } },
    ],
    connections: [
      { id: "ci", kind: "ci", name: "CI", url: "", browserUrl: "", auth: "none", username: "", caFile: "", rules: [], targets: [{ id: targetID, name: targetName, service: "", environment: "", capabilities: ["builds"] }] },
      { id: "observe", kind: "observability", name: "Observe", url: "", browserUrl: "", auth: "none", username: "", caFile: "", targets: [], rules: [{ id: "latency", name: ruleName, expression: "x".repeat(200), threshold: 1, unit, description: "", enabled: true }, { id: "memory", name: "Memory", expression: "memory", threshold: 1, unit: "%", description: "", enabled: true }, { id: "disk", name: "Disk", expression: "disk", threshold: 1, unit: "%", description: "", enabled: true }] },
    ],
  };
  const layout = defaultLayout(state);
  expect(layout.widgets.map(item => item.kind)).toEqual(["builds", "queue", "metrics", "metrics"]);
  expect(layout.widgets[0].targetId).toBe(targetID);
  expect(layout.widgets[1].targetId).toBe(targetID);
  expect(layout.widgets[0].title).toHaveLength(80);
  expect(layout.widgets[2].title).toHaveLength(80);
  expect(layout.widgets[2].expression).toHaveLength(200);
  expect(layout.widgets[2].refreshSeconds).toBe(60);
  expect(layout.widgets[2].unit).toHaveLength(24);
  expect(layout.widgets[3].expression).toBe("memory");
  expect(() => parseLayout(JSON.stringify(layout))).not.toThrow();
});

test("an explicitly empty saved layout stays empty", () => {
  expect(parseLayout('{"version":1,"widgets":[]}')).toEqual({ version: 1, widgets: [] });
});
