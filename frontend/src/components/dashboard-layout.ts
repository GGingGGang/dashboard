import type { ExtensionBinding } from "../bridge/extensions";
import type { State } from "../shared/types";
import { validateJSONRequest, type JSONRequest } from "../shared/json-query";

export type WidgetKind = "builds" | "queue" | "deployments" | "metrics" | "logs" | "traces" | "json" | "extension";

export type DashboardWidget = {
  id: string;
  kind: WidgetKind;
  title: string;
  connectionId: string;
  targetId: string;
  expression: string;
  unit: string;
  period: number;
  refreshSeconds: number;
  x: number;
  y: number;
  w: number;
  h: number;
  json?: JSONRequest;
  extension?: ExtensionBinding;
};

export type DashboardLayout = { version: 2; widgets: DashboardWidget[] };

export const LAYOUT_KEY = "dashboard.layout";
export const MAX_WIDGETS = 24;
export const COLUMNS = 12;

const MAX_PREFERENCE_BYTES = 1024 * 1024;
const MAX_Y = 500;
const kinds = new Set<WidgetKind>(["builds", "queue", "deployments", "metrics", "logs", "traces", "json", "extension"]);
const periods = new Set([900, 3600, 21600, 86400]);
const refreshes = new Set([0, 30, 60, 120]);
const widgetKeys = new Set<keyof DashboardWidget>([
  "id", "kind", "title", "connectionId", "targetId", "expression", "unit", "period", "refreshSeconds", "x", "y", "w", "h",
]);

type RecordValue = Record<string, unknown>;

function fail(message: string): never { throw new Error(`대시보드 레이아웃: ${message}`); }
function plain(value: unknown): value is RecordValue {
  return typeof value === "object" && value !== null && !Array.isArray(value) && Object.getPrototypeOf(value) === Object.prototype;
}
function string(value: unknown, field: string, max: number): string {
  if (typeof value !== "string" || value.length > max) fail(`${field} 값이 올바르지 않습니다`);
  return value;
}
function integer(value: unknown, field: string): number {
  if (!Number.isInteger(value)) fail(`${field} 값은 정수여야 합니다`);
  return value as number;
}
function sameKeys(value: RecordValue, expected: Set<string>, name: string) {
  if (Object.keys(value).length !== expected.size || Object.keys(value).some(key => !expected.has(key))) fail(`${name} 필드가 올바르지 않습니다`);
}
function validateWidget(value: unknown): DashboardWidget {
  if (!plain(value)) fail("위젯 형식이 올바르지 않습니다");
  sameKeys(value, value.kind === "json" ? new Set([...widgetKeys, "json"]) : value.kind === "extension" ? new Set([...widgetKeys, "extension"]) : widgetKeys, "위젯");
  const widget: DashboardWidget = {
    id: string(value.id, "id", 120), kind: string(value.kind, "kind", 20) as WidgetKind,
    title: string(value.title, "title", 120), connectionId: string(value.connectionId, "connectionId", 120),
    targetId: string(value.targetId, "targetId", 16384), expression: string(value.expression, "expression", 16384),
    unit: string(value.unit, "unit", 40), period: integer(value.period, "period"),
    refreshSeconds: integer(value.refreshSeconds, "refreshSeconds"), x: integer(value.x, "x"),
    y: integer(value.y, "y"), w: integer(value.w, "w"), h: integer(value.h, "h"),
  };
  if (!widget.id.trim()) fail("위젯 id가 비어 있습니다");
  if (!kinds.has(widget.kind)) fail("지원하지 않는 위젯 종류입니다");
  if (!periods.has(widget.period)) fail("지원하지 않는 조회 기간입니다");
  if (!refreshes.has(widget.refreshSeconds)) fail("지원하지 않는 새로고침 주기입니다");
  if (widget.kind === "extension") {
    const binding = value.extension;
    if (!plain(binding)) fail("플러그인 카드 바인딩이 올바르지 않습니다");
    sameKeys(binding, new Set(["pluginId", "kind", "id", "sourceId", "input"]), "플러그인 바인딩");
    for (const key of ["pluginId", "id", "sourceId"]) if (!/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$/.test(string(binding[key], key, 128))) fail("플러그인 ID가 올바르지 않습니다");
    if ((binding.kind !== "card" && binding.kind !== "query") || !plain(binding.input) || JSON.stringify(binding.input).length > 65536) fail("플러그인 입력이 올바르지 않습니다");
    widget.extension = structuredClone(binding) as ExtensionBinding;
  }
  if (widget.kind === "json") {
    widget.json = validateJSONRequest(value.json);
    if (widget.refreshSeconds !== 0) fail("API 노드는 수동으로 조회합니다");
  }
  if (widget.x < 0 || widget.y < 0 || widget.y > MAX_Y || widget.w < 3 || widget.h < 2 || widget.h > 8 || widget.x + widget.w > COLUMNS) fail("위젯 위치 또는 크기가 화면 범위를 벗어났습니다");
  return widget;
}

function checkedWidgets(widgets: DashboardWidget[]): DashboardWidget[] {
  if (!Array.isArray(widgets) || widgets.length > MAX_WIDGETS) fail(`위젯은 최대 ${MAX_WIDGETS}개까지 저장할 수 있습니다`);
  const ids = new Set<string>();
  return widgets.map(widget => {
    const checked = validateWidget(widget);
    if (ids.has(checked.id)) fail("위젯 id가 중복되었습니다");
    ids.add(checked.id);
    return checked;
  });
}

function rejectOverlaps(widgets: DashboardWidget[]) {
  for (let left = 0; left < widgets.length; left++) for (let right = left + 1; right < widgets.length; right++) {
    if (overlaps(widgets[left], widgets[right])) fail("저장된 위젯 위치가 서로 겹칩니다");
  }
}

export function parseLayout(raw: string): DashboardLayout | null {
  if (!raw.trim()) return null;
  if (new TextEncoder().encode(raw).byteLength > MAX_PREFERENCE_BYTES) fail("저장된 값이 너무 큽니다");
  let value: unknown;
  try { value = JSON.parse(raw); } catch { fail("저장된 JSON을 읽을 수 없습니다"); }
  if (!plain(value)) fail("저장된 형식이 올바르지 않습니다");
  sameKeys(value, new Set(["version", "widgets"]), "레이아웃");
  if (value.version !== 1 && value.version !== 2) fail("이 앱이 지원하지 않는 레이아웃 버전입니다");
  if (!Array.isArray(value.widgets)) fail("widgets 값이 배열이 아닙니다");
  const widgets = checkedWidgets(value.widgets);
  rejectOverlaps(widgets);
  return { version: 2, widgets };
}

function overlaps(a: DashboardWidget, b: DashboardWidget) {
  return a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y;
}

export function nextPosition(widgets: DashboardWidget[], w: number, h: number): { x: number; y: number } {
  if (!Number.isInteger(w) || !Number.isInteger(h) || w < 3 || h < 2 || h > 8 || w > COLUMNS) fail("새 위젯 크기가 올바르지 않습니다");
  const existing = checkedWidgets(widgets);
  for (let y = 0; y <= MAX_Y; y++) for (let x = 0; x + w <= COLUMNS; x++) {
    const candidate = { x, y, w, h } as DashboardWidget;
    if (!existing.some(widget => overlaps(candidate, widget))) return { x, y };
  }
  fail("새 위젯을 배치할 공간이 없습니다");
}

export function placeWidget(widgets: DashboardWidget[], changed: DashboardWidget): DashboardWidget[] {
  const existing = checkedWidgets(widgets);
  const next = validateWidget(changed);
  const withoutChanged = existing.filter(widget => widget.id !== next.id);
  if (withoutChanged.length === existing.length && existing.length >= MAX_WIDGETS) fail(`위젯은 최대 ${MAX_WIDGETS}개까지 저장할 수 있습니다`);
  const placed: DashboardWidget[] = [next];
  for (const widget of [...withoutChanged].sort((left, right) => left.y - right.y || left.x - right.x || left.id.localeCompare(right.id))) {
    let candidate = { ...widget };
    while (true) {
      const collisions = placed.filter(other => overlaps(candidate, other));
      if (!collisions.length) break;
      candidate.y = Math.max(...collisions.map(other => other.y + other.h));
      if (candidate.y > MAX_Y) fail("위젯을 아래로 이동할 공간이 없습니다");
    }
    placed.push(candidate);
  }
  const order = existing.some(widget => widget.id === next.id) ? existing : [...existing, next];
  return order.map(widget => ({ ...(placed.find(placedWidget => placedWidget.id === widget.id) || next) }));
}

function supports(state: State, kind: WidgetKind, connectionId: string, targetId = "") {
  const connection = state.connections.find(item => item.id === connectionId);
  const provider = connection && state.providers.find(item => item.kind === connection.kind);
  const target = targetId && connection?.targets.find(item => item.id === targetId);
  return !!provider?.capabilities.includes(kind) && (!target || !target.capabilities?.length || target.capabilities.includes(kind) || (kind === "queue" && target.capabilities.includes("builds")));
}

const dimensions: Record<WidgetKind, [number, number]> = { builds: [4, 3], queue: [4, 3], deployments: [4, 3], metrics: [4, 4], logs: [6, 4], traces: [6, 4], json: [4, 3], extension: [6, 4] };
const label = (value: string, max: number) => value.slice(0, max);

export function defaultLayout(state: State): DashboardLayout {
  const widgets: DashboardWidget[] = [];
  const add = (kind: WidgetKind, connectionId: string, title: string, targetId = "", expression = "", unit = "") => {
    if (widgets.length >= 8 || !supports(state, kind, connectionId, targetId)) return;
    const [w, h] = dimensions[kind];
    const position = nextPosition(widgets, w, h);
    widgets.push({ id: `widget-${widgets.length + 1}`, kind, title: label(title, 80), connectionId, targetId, expression, unit: label(unit, 24), period: 3600, refreshSeconds: kind === "metrics" ? 60 : 30, ...position, w, h });
  };
  for (const connection of state.connections) {
    const targets = connection.targets || [];
    const target = targets.find(item => supports(state, "builds", connection.id, item.id));
    if (target) add("builds", connection.id, `${target.name} 빌드`, target.id);
    const deployment = targets.find(item => supports(state, "deployments", connection.id, item.id));
    if (deployment) add("deployments", connection.id, `${deployment.name} 배포`, deployment.id);
    for (const rule of (connection.rules || []).filter(item => item.enabled).slice(0, 2)) add("metrics", connection.id, rule.name, "", rule.expression, rule.unit);
    const snapshot = state.snapshots.find(item => item.connectionId === connection.id);
    const queue = targets.find(item => supports(state, "queue", connection.id, item.id) && snapshot?.queue.some(row => row.job === item.id));
    if (queue) add("queue", connection.id, `${queue.name} 대기열`, queue.id);
  }
  return { version: 2, widgets };
}
