import { api } from "../bridge";
import { badge, date, elapsed, esc } from "../shared/format";
import type {
  Build,
  Connection,
  LogResult,
  Observation,
  QueryResult,
  Snapshot,
  State,
  TraceSearchResult,
} from "../shared/types";
import type { DashboardWidget } from "./dashboard-layout";
import type { JSONResult } from "../shared/json-query";

type RemoteResult =
  | { kind: "json"; value: JSONResult }
  | { kind: "metrics"; value: QueryResult }
  | { kind: "logs"; value: LogResult }
  | { kind: "traces"; value: TraceSearchResult };
type Entry = {
  key: string;
  token: number;
  loading: boolean;
  updated?: number;
  result?: RemoteResult;
  error?: string;
};
type GraphView = { scale: number; left: number };
const fullGraph: GraphView = { scale: 1, left: 0 };
const clamp = (value: number, low: number, high: number) => Math.max(low, Math.min(high, value));

const REMOTE = new Set<DashboardWidget["kind"]>(["metrics", "logs", "traces", "json"]);
const MAX_PERIOD = 24 * 60 * 60;
const maxPeriod = (value: number) => Math.min(MAX_PERIOD, Math.max(1, Number(value) || 3600));
const connectionKey = (connection: Connection | undefined) =>
  connection
    ? JSON.stringify([connection.id, connection.revision, connection.kind, connection.url, connection.tenant])
    : "missing";
const labels = (value: Record<string, string>) =>
  Object.entries(value || {}).map(([key, item]) => `${key}=${item}`).join(", ") || "value";
const number = (value: number) => new Intl.NumberFormat("ko-KR", { maximumSignificantDigits: 4 }).format(value);
const snapshotFor = (state: State, id: string) => state.snapshots.find((snapshot) => snapshot.connectionId === id);
const connectionFor = (state: State, id: string) => state.connections.find((connection) => connection.id === id);
const supports = (state: State, connection: Connection, capability: string) =>
  state.providers.find((provider) => provider.kind === connection.kind)?.capabilities.includes(capability) || false;
const targetFor = (connection: Connection, id: string) => connection.targets.find((target) => target.id === id);
const targetSupports = (target: Connection["targets"][number] | undefined, kind: DashboardWidget["kind"]) =>
  !target?.capabilities?.length || target.capabilities.includes(kind) || (kind === "queue" && target.capabilities.includes("builds"));
const sourceStatus = (state: State, connectionId: string, snapshot: Snapshot | undefined, capability?: string) => {
  const source = capability && snapshot?.modules ? snapshot.modules[capability] : snapshot;
  if (!source) return "아직 수집된 정보가 없습니다.";
  if (source.error) return `수집 오류: ${source.error}`;
  const poll = state.providers.find((provider) => provider.kind === connectionFor(state, connectionId)?.kind)?.pollSeconds || 30;
  const at = Date.parse(source.lastSuccess);
  return Number.isFinite(at) && Date.now() - at > Math.max(30, poll * 2) * 1000
    ? `수집 지연 · 마지막 수집 ${date(source.lastSuccess)}`
    : `마지막 수집 ${date(source.lastSuccess)}`;
};
const observationStatus = (observation: Observation) => {
  const completeness = observation.completeness === "complete" ? "전체 범위" : observation.completeness === "truncated" ? "일부 결과" : "전체 여부 미확인";
  return `관측 ${date(observation.observedAt)} · ${completeness}`;
};
const nsDate = (value: string) => {
  try { return date(Number(BigInt(value) / 1000000n)); } catch { return value || "시각 없음"; }
};

/** Keeps remote dashboard widgets bounded and ignores replies from an old layout. */
export class DashboardData {
  private entries = new Map<string, Entry>();
  private graphViews = new Map<string, GraphView>();
  private widgets = new Map<string, DashboardWidget>();
  private timers = new Map<string, { handle: number; seconds: number }>();
  private mounted = new Set<string>();
  private queued = new Set<string>();
  private queue: string[] = [];
  private active = 0;
  private stopped = false;

  constructor(private readonly getState: () => State, private readonly notify: (widgetId: string) => void) {}

  update(widgets: DashboardWidget[]) {
    if (this.stopped) return;
    const next = new Map(widgets.map((widget) => [widget.id, widget]));
    for (const id of this.widgets.keys()) {
      if (!next.has(id)) this.remove(id);
    }
    this.widgets = next;
    for (const widget of widgets) {
      const key = this.key(widget);
      const old = this.entries.get(widget.id);
      if (!old || old.key !== key) {
        if (old) old.token++;
        this.graphViews.delete(widget.id);
        this.entries.set(widget.id, { key, token: (old?.token || 0) + 1, loading: false });
        if (REMOTE.has(widget.kind) && widget.kind !== "json") this.enqueue(widget.id);
      }
      this.schedule(widget);
    }
  }

  attach(widgetId: string, element: HTMLElement) {
    if (this.stopped || !this.widgets.has(widgetId)) return;
    this.mounted.add(widgetId);
    const widget = this.widgets.get(widgetId)!;
    this.schedule(widget);
    element.onpointermove = event => {
      const graph = element.querySelector<SVGElement>(".db-chart");
      const tooltip = element.querySelector<HTMLElement>(".db-chart-tooltip");
      const cursor = element.querySelector<HTMLElement>(".db-chart-cursor");
      if (!graph || !tooltip || !cursor) return;
      const bounds = graph.getBoundingClientRect();
      const inside = event.clientX >= bounds.left && event.clientX <= bounds.right && event.clientY >= bounds.top && event.clientY <= bounds.bottom;
      tooltip.hidden = cursor.hidden = !inside;
      if (!inside) return;
      const result = this.entries.get(widgetId)?.result;
      if (result?.kind !== "metrics") return;
      const fraction = clamp((event.clientX - bounds.left) / bounds.width, 0, 1);
      const time = Number(graph.dataset.start) + fraction * (Number(graph.dataset.end) - Number(graph.dataset.start));
      tooltip.textContent = result.value.series.slice(0, 6).map(item => {
        const point = item.points.reduce<(typeof item.points)[number] | undefined>((best, point) => !best || Math.abs(point.time - time) < Math.abs(best.time - time) ? point : best, undefined);
        return `${labels(item.labels)} · ${point ? `${date(point.time * 1000)} · ${point.value === null || !Number.isFinite(point.value) ? "값 없음" : number(point.value) + widget.unit}` : "값 없음"}`;
      }).join("\n");
      cursor.style.left = `${fraction * 100}%`;
    };
    element.onpointerleave = () => element.querySelectorAll<HTMLElement>(".db-chart-tooltip,.db-chart-cursor").forEach(el => el.hidden = true);
  }

  detach(widgetId: string) {
    this.mounted.delete(widgetId);
    this.clearTimer(widgetId);
  }

  zoomGraph(widgetId: string, multiplier: number | null, anchorX = 0.5) {
    if (this.stopped || this.widgets.get(widgetId)?.kind !== "metrics") return;
    const old = this.graphViews.get(widgetId) || fullGraph;
    const scale = multiplier === null ? 1 : clamp(old.scale * multiplier, 1, 8);
    const span = 1 / scale;
    const view = {
      scale,
      left: clamp(old.left + clamp(anchorX, 0, 1) * (1 / old.scale - span), 0, 1 - span),
    };
    this.graphViews.set(widgetId, view);
    this.notify(widgetId);
  }

  refresh(widgetId?: string) {
    if (this.stopped) return;
    if (widgetId) {
      const widget = this.widgets.get(widgetId);
      if (widget && REMOTE.has(widget.kind)) this.enqueue(widgetId);
      return;
    }
    for (const widget of this.widgets.values()) if (REMOTE.has(widget.kind) && widget.kind !== "json") this.enqueue(widget.id);
  }

  seedJSON(id: string, value: JSONResult) {
    const entry = this.entries.get(id);
    if (entry && this.widgets.get(id)?.kind === "json") {
      entry.result = { kind: "json", value };
      entry.updated = Date.parse(value.observedAt);
    }
  }

  stop() {
    this.stopped = true;
    for (const entry of this.entries.values()) entry.token++;
    for (const timer of this.timers.values()) clearTimeout(timer.handle);
    this.timers.clear();
    this.queue = [];
    this.queued.clear();
    this.mounted.clear();
    this.graphViews.clear();
  }

  markup(widget: DashboardWidget) {
    if (widget.kind === "json") return this.remoteMarkup(widget);
    const state = this.getState();
    const connection = connectionFor(state, widget.connectionId);
    if (!connection) return this.notice("연결이 없습니다. 카드 설정에서 연결을 선택하세요.", "error");
    if (!supports(state, connection, widget.kind)) return this.notice(`${connection.name} 연결은 ${widget.kind} 데이터를 제공하지 않습니다.`, "warning");
    const target = widget.targetId ? targetFor(connection, widget.targetId) : undefined;
    if (widget.targetId && !target) return this.notice("선택한 대상이 이 연결에 없습니다.", "error");
    if (!targetSupports(target, widget.kind)) return this.notice("선택한 대상은 이 데이터를 제공하지 않습니다.", "warning");
    if (REMOTE.has(widget.kind) && !widget.expression.trim()) return this.notice("조회식이 비어 있습니다. 카드 설정에서 입력하세요.", "warning");
    if (widget.kind === "builds" || widget.kind === "queue" || widget.kind === "deployments") {
      return this.snapshotMarkup(widget, snapshotFor(state, connection.id));
    }
    return this.remoteMarkup(widget);
  }

  private remove(id: string) {
    const entry = this.entries.get(id);
    if (entry) entry.token++;
    this.entries.delete(id);
    this.graphViews.delete(id);
    this.mounted.delete(id);
    this.clearTimer(id);
    this.queued.delete(id);
    this.queue = this.queue.filter((queued) => queued !== id);
  }

  private key(widget: DashboardWidget) {
    if (widget.kind === "json") return JSON.stringify(widget.json);
    return JSON.stringify([widget.kind, widget.connectionId, widget.targetId, widget.expression, maxPeriod(widget.period), connectionKey(connectionFor(this.getState(), widget.connectionId))]);
  }

  private schedule(widget: DashboardWidget) {
    if (widget.kind === "json" || !REMOTE.has(widget.kind) || !this.mounted.has(widget.id) || !widget.refreshSeconds) {
      this.clearTimer(widget.id);
      return;
    }
    if (this.timers.get(widget.id)?.seconds === widget.refreshSeconds) return;
    this.clearTimer(widget.id);
    const handle = window.setTimeout(() => {
      this.timers.delete(widget.id);
      const current = this.widgets.get(widget.id);
      if (!this.stopped && current && this.mounted.has(widget.id) && this.entries.get(widget.id)?.key === this.key(current)) {
        this.enqueue(current.id);
        this.schedule(current);
      }
    }, widget.refreshSeconds * 1000);
    this.timers.set(widget.id, { handle, seconds: widget.refreshSeconds });
  }

  private clearTimer(id: string) {
    const timer = this.timers.get(id);
    if (timer !== undefined) clearTimeout(timer.handle);
    this.timers.delete(id);
  }

  private enqueue(id: string) {
    const entry = this.entries.get(id);
    if (!entry || entry.loading || this.queued.has(id)) return;
    this.queued.add(id);
    this.queue.push(id);
    this.pump();
  }

  private pump() {
    while (!this.stopped && this.active < 2 && this.queue.length) {
      const id = this.queue.shift()!;
      this.queued.delete(id);
      const widget = this.widgets.get(id);
      const entry = this.entries.get(id);
      if (!widget || !entry || entry.loading || !REMOTE.has(widget.kind)) continue;
      void this.run(widget, entry);
    }
  }

  private async run(widget: DashboardWidget, entry: Entry) {
    const state = this.getState();
    const connection = connectionFor(state, widget.connectionId);
    const target = connection && widget.targetId ? targetFor(connection, widget.targetId) : undefined;
    if (widget.kind === "json" ? !widget.json : (!connection || !supports(state, connection, widget.kind) || !widget.expression.trim() || (widget.targetId && !target) || !targetSupports(target, widget.kind))) return;
    const token = ++entry.token;
    const key = entry.key;
    entry.loading = true;
    entry.error = undefined;
    this.active++;
    this.notify(widget.id);
    const end = Math.floor(Date.now() / 1000);
    const request = { expression: widget.expression, start: end - maxPeriod(widget.period), end };
    try {
      const value = widget.kind === "json"
        ? await api.ReadJSON(widget.json!)
        : widget.kind === "metrics"
        ? await api.Query(connection!.id, request)
        : widget.kind === "logs"
          ? await api.SearchLogs(connection!.id, { ...request, limit: 100 })
          : await api.SearchTraces(connection!.id, { ...request, limit: 30 });
      const current = this.entries.get(widget.id);
      const latest = this.widgets.get(widget.id);
      if (!this.stopped && current === entry && current.token === token && current.key === key && latest && this.key(latest) === key) {
        current.result = { kind: widget.kind, value } as RemoteResult;
        current.updated = Date.now();
        current.error = undefined;
        this.notify(widget.id);
      }
    } catch (error) {
      const current = this.entries.get(widget.id);
      const latest = this.widgets.get(widget.id);
      if (!this.stopped && current === entry && current.token === token && current.key === key && latest && this.key(latest) === key) {
        current.error = String(error);
        this.notify(widget.id);
      }
    } finally {
      const current = this.entries.get(widget.id);
      if (current === entry && current.token === token) {
        current.loading = false;
        if (!this.stopped) this.notify(widget.id);
      }
      this.active--;
      this.pump();
    }
  }

  private snapshotMarkup(widget: DashboardWidget, snapshot: Snapshot | undefined) {
    const target = widget.targetId;
    const state = this.getState();
    const connection = connectionFor(state, widget.connectionId);
    const meta = `<p class="db-meta">출처 ${esc(connection?.name || widget.connectionId)} · ${esc(sourceStatus(state, widget.connectionId, snapshot, widget.kind))}</p>`;
    if (!snapshot) return `${meta}${this.notice("이 연결의 수집 결과를 기다리고 있습니다.", "warning")}`;
    if (widget.kind === "builds") {
      const all = snapshot.builds.filter((build) => !target || build.job === target);
      const builds = all.slice(0, 20);
      return `${meta}${builds.length ? `<div class="db-table"><table><thead><tr><th>작업</th><th>결과</th><th>시작</th></tr></thead><tbody>${builds.map((build) => this.buildRow(build)).join("")}</tbody></table></div>${this.clipped(all.length)}` : this.empty("표시할 빌드가 없습니다.")}`;
    }
    if (widget.kind === "queue") {
      const all = snapshot.queue.filter((item) => !target || item.job === target);
      const rows = all.slice(0, 20);
      return `${meta}${rows.length ? `<div class="db-table"><table><thead><tr><th>작업</th><th>대기</th><th>사유</th></tr></thead><tbody>${rows.map((item) => `<tr><td>${esc(item.job)}</td><td>${esc(elapsed(Date.now() - item.since))}</td><td>${esc(item.reason || "—")}</td></tr>`).join("")}</tbody></table></div>${this.clipped(all.length)}` : this.empty("대기 중인 작업이 없습니다.")}`;
    }
    const all = snapshot.deployments.filter((item) => !target || item.id === target);
    const rows = all.slice(0, 20);
    return `${meta}${rows.length ? `<div class="db-table"><table><thead><tr><th>서비스</th><th>동기화</th><th>상태</th></tr></thead><tbody>${rows.map((item) => `<tr><td>${esc(item.name)}<br><small>${esc(item.revisions.join(", ") || "revision 없음")}</small></td><td>${badge(item.sync)}</td><td>${badge(item.health)}<br><small>${esc(item.phase || "—")}</small></td></tr>`).join("")}</tbody></table></div>${this.clipped(all.length)}` : this.empty("표시할 배포가 없습니다.")}`;
  }

  private buildRow(build: Build) {
    return `<tr><td>${esc(build.job)} #${esc(build.number)}</td><td>${badge(build.status)}</td><td>${esc(date(build.started))}</td></tr>`;
  }

  private remoteMarkup(widget: DashboardWidget) {
    const entry = this.entries.get(widget.id);
    const connection = connectionFor(this.getState(), widget.connectionId);
    const source = `<p class="db-meta">출처 ${esc(widget.kind === "json" ? widget.json?.url || "HTTP API" : connection?.name || widget.connectionId)} · 직접 조회${entry?.updated ? ` · 마지막 응답 ${esc(date(entry.updated))}` : ""}</p>`;
    if (!entry?.result) {
      return `${source}${entry?.loading ? this.notice("조회 중…", "info") : ""}${entry?.error ? this.notice(entry.error, "error") : this.empty("아직 조회된 데이터가 없습니다.")}`;
    }
    const stale = entry.error ? this.notice(`마지막 결과를 표시합니다. 새 조회 실패: ${entry.error}`, "warning") : "";
    const pending = entry.loading ? `<span class="db-loading">새로고침 중…</span>` : "";
    const result = entry.result;
    const body = result.kind === "json" ? `${result.value.scalar ? `<div class="db-current">${esc(result.value.text)}</div>` : `<pre class="db-json">${esc(result.value.json)}</pre>`}<details><summary>조회 결과 JSON · ${result.value.count}개</summary><pre class="db-json">${esc(result.value.json)}</pre></details>` : result.kind === "metrics" ? this.metrics(result.value, widget.unit, this.graphViews.get(widget.id) || fullGraph) : result.kind === "logs" ? this.logs(result.value) : this.traces(result.value);
    return `${source}${stale}${pending}${body}`;
  }

  private metrics(result: QueryResult, unit: string, view: GraphView) {
    const series = result.series || [];
    const shown = series.slice(0, 6);
    const warning = series.length > shown.length ? this.notice(`${series.length}개 라벨 시계열 중 처음 ${shown.length}개만 그래프로 표시합니다. 값은 합산하지 않았습니다.`, "warning") : "";
    const latest = (item: QueryResult["series"][number]): { time: number; value: number } | undefined => {
      for (let index = item.points.length - 1; index >= 0; index--) {
        const point = item.points[index];
        if (point.value !== null && Number.isFinite(point.value)) return { time: point.time, value: point.value };
      }
      return undefined;
    };
    const graph = this.graph(shown, unit, view);
    const colors = ["#73e0db", "#9ebcff", "#f8cb7c", "#ffabb8", "#afc996", "#d8a4ff"];
    return `${warning}${graph}<ul class="db-series">${shown.map((item, index) => { const point = latest(item); return `<li><span class="db-series-swatch" style="background:${colors[index]}"></span><code>${esc(labels(item.labels))}</code> · ${esc(point ? date(point.time * 1000) : "값 없음")}</li>`; }).join("")}</ul>${result.warnings?.length ? this.notice(result.warnings.join(" · "), "warning") : ""}${result.truncated ? this.notice("응답이 잘렸습니다.", "warning") : ""}`;
  }

  private graph(series: QueryResult["series"], unit: string, view: GraphView) {
    const points = series.flatMap((item) => item.points.filter((point) => point.value !== null && Number.isFinite(point.value)).map((point) => ({ ...point, value: point.value as number })));
    if (!points.length) return this.empty("표시할 숫자 데이터가 없습니다.");
    const low = Math.min(...points.map((point) => point.value));
    const high = Math.max(...points.map((point) => point.value));
    const start = Math.min(...points.map((point) => point.time));
    const end = Math.max(...points.map((point) => point.time));
    const valuePadding = high === low ? Math.max(Math.abs(high) * 0.1, 0.001) : 0;
    const timeSpan = Math.max(1, end - start);
    const viewStart = start - (end === start ? 0.5 : 0) + view.left * timeSpan;
    const viewEnd = viewStart + timeSpan / view.scale;
    const viewHigh = high + valuePadding;
    const viewLow = low - valuePadding;
    const x = (time: number) => ((time - viewStart) / (viewEnd - viewStart)) * 200;
    const y = (value: number) => ((viewHigh - value) / (viewHigh - viewLow)) * 70;
    const colors = ["#73e0db", "#9ebcff", "#f8cb7c", "#ffabb8", "#afc996", "#d8a4ff"];
    return `<div class="db-chart-scale"><span>값 범위 ${esc(number(viewLow))}–${esc(number(viewHigh))}${esc(unit)}</span><div class="db-chart-controls" role="group" aria-label="시간축 확대·축소"><button type="button" data-graph-action="out" aria-label="시간축 축소" ${view.scale === 1 ? "disabled" : ""}>−</button><button type="button" data-graph-action="reset" aria-label="전체 시간 범위" title="전체 시간 범위로 돌아가기">${Math.round(view.scale * 100)}%</button><button type="button" data-graph-action="in" aria-label="시간축 확대" ${view.scale === 8 ? "disabled" : ""}>＋</button></div></div><div class="db-chart-frame"><div class="db-y-axis">${[viewHigh, (viewHigh + viewLow) / 2, viewLow].map((value, index) => `<span style="top:${index * 50}%">${esc(number(value) + unit)}</span>`).join("")}</div><div class="db-plot"><svg class="db-chart" data-start="${viewStart}" data-end="${viewEnd}" viewBox="0 0 200 70" preserveAspectRatio="none" role="img" aria-label="${esc(`지표 추이 ${date(viewStart * 1000)}부터 ${date(viewEnd * 1000)}까지`)}"><title>Ctrl + 휠로 마우스 위치의 시간축만 확대·축소합니다.</title>${[0.5, 35, 69.5].map(y => `<line x1="0" x2="200" y1="${y}" y2="${y}" stroke="#40536b" stroke-dasharray="2 3" vector-effect="non-scaling-stroke"/>`).join("")}${series.map((item, index) => {
      let path = "";
      let dots = "";
      let open = false;
      for (const [pointIndex, point] of item.points.entries()) {
        if (point.value === null || !Number.isFinite(point.value)) { open = false; continue; }
        path += `${open ? "L" : "M"}${x(point.time).toFixed(2)},${y(point.value as number).toFixed(2)}`;
        const next = item.points[pointIndex + 1];
        if (!open && (!next || next.value === null || !Number.isFinite(next.value))) {
          dots += `<circle cx="${x(point.time).toFixed(2)}" cy="${y(point.value).toFixed(2)}" r="1.5" fill="${colors[index]}"/>`;
        }
        open = true;
      }
      return `<path d="${path}" fill="none" stroke="${colors[index]}" stroke-width="2.2" vector-effect="non-scaling-stroke"/>${dots}`;
    }).join("")}</svg><div class="db-chart-cursor" hidden></div><div class="db-chart-tooltip" role="tooltip" hidden></div></div><div class="db-end-values">${series.map(item => {
      const point = item.points.filter(point => point.time >= viewStart && point.time <= viewEnd && point.value !== null && Number.isFinite(point.value)).at(-1);
      return point ? `<span class="db-current" style="top:${y(point.value!) / 70 * 100}%" title="${esc(labels(item.labels) + ' · ' + date(point.time * 1000))}">${esc(number(point.value!) + unit)}</span>` : "";
    }).join("")}</div></div><div class="db-chart-axis"><span>${esc(date(viewStart * 1000))}</span><span>${esc(date(viewEnd * 1000))}</span></div>`;
  }

  private logs(result: LogResult) {
    const meta = `<p class="db-meta">${esc(observationStatus(result.observation))}</p>`;
    return `${meta}${this.observationNotes(result.observation)}${result.entries.length ? `<div class="db-table"><table><thead><tr><th>시각</th><th>로그</th></tr></thead><tbody>${result.entries.map((item) => `<tr><td>${esc(nsDate(item.timestampNs))}</td><td><pre>${esc(item.line)}</pre><small>${esc(labels(item.labels))}</small></td></tr>`).join("")}</tbody></table></div>` : this.empty("요청 범위의 로그가 없습니다.")}`;
  }

  private traces(result: TraceSearchResult) {
    const meta = `<p class="db-meta">${esc(observationStatus(result.observation))}</p>`;
    return `${meta}${this.observationNotes(result.observation)}${result.traces.length ? `<div class="db-table"><table><thead><tr><th>서비스</th><th>작업</th><th>소요</th></tr></thead><tbody>${result.traces.map((item) => `<tr><td>${esc(item.rootService || "미제공")}</td><td><code>${esc(item.rootName || "미제공")}</code><br><small>${esc(item.traceId)}</small></td><td>${esc(String(item.durationMs))} ms</td></tr>`).join("")}</tbody></table></div>` : this.empty("요청 범위의 트레이스가 없습니다.")}`;
  }

  private observationNotes(observation: Observation) {
    return observation.diagnostics?.length ? this.notice(observation.diagnostics.join(" · "), observation.outcome === "error" ? "error" : "warning") : "";
  }

  private clipped(count: number) { return count > 20 ? this.notice(`${count}개 중 처음 20개만 표시합니다.`, "warning") : ""; }
  private empty(message: string) { return `<p class="db-empty">${esc(message)}</p>`; }
  private notice(message: string, kind: "error" | "warning" | "info") { return `<p class="db-notice db-${kind}" role="status">${esc(message)}</p>`; }
}
