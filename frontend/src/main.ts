import * as echarts from "echarts/core";
import { LineChart } from "echarts/charts";
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
} from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type {
  Bridge,
  Build,
  Connection,
  QueryResult,
  Rule,
  Snapshot,
  State,
  Target,
} from "./types";
import { previewBridge } from "./preview";
import "./style.css";

echarts.use([
  LineChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  CanvasRenderer,
]);
const api: Bridge =
  window.go?.main.App ??
  previewBridge(new URLSearchParams(location.search).has("empty"));
const browserPreview = !window.go;
let state: State = {
  connections: [],
  snapshots: [],
  providers: [],
  error: "",
  demo: browserPreview,
};
let page = "overview",
  scope = "*",
  period = 3600,
  historyOffset = 0,
  editing: Connection | null = null,
  discovered: Target[] = [];
let charts: echarts.ECharts[] = [],
  pollBusy = false,
  renderedStamp = "",
  generation = 0;
let favorites: { name: string; expression: string }[] = [];
let queryExpression = "up",
  queryConnection = "";
let chartCache = new Map<string, { result: QueryResult; at: number }>();
const esc = (s: unknown) =>
  String(s ?? "").replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ]!,
  );
const date = (v: number | string) =>
  !v || new Date(v).getFullYear() < 2000
    ? "아직 없음"
    : new Date(v).toLocaleString("ko-KR", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false,
      });
const elapsed = (ms: number) =>
  `${Math.floor(Math.max(0, ms) / 60000)}분 ${Math.floor(Math.max(0, ms) / 1000) % 60}초`;
const cls = (status: string) =>
  ["SUCCESS", "Healthy", "Synced", "Succeeded"].includes(status)
    ? "good"
    : ["FAILURE", "Failed", "Error", "Degraded"].includes(status)
      ? "bad"
      : ["RUNNING", "Running", "Progressing"].includes(status)
        ? "active"
        : ["UNSTABLE", "OutOfSync", "Missing", "Suspended"].includes(status)
          ? "warn"
          : "muted";
const badge = (status: string) =>
  `<span class="badge ${cls(status)}">${esc(status || "Unknown")}</span>`;
const snap = (id: string) => state.snapshots.find((s) => s.connectionId === id);
const conn = (id: string) => state.connections.find((c) => c.id === id);
const supports = (c: Connection | null | undefined, capability: string) =>
  !!state.providers
    .find((p) => p.kind === c?.kind)
    ?.capabilities.includes(capability);
const fresh = (s: Snapshot | undefined) =>
  !!s &&
  !s.error &&
  new Date(s.lastSuccess).getFullYear() > 2000 &&
  Date.now() - Date.parse(s.lastSuccess) <
    (supports(conn(s.connectionId), "rules") ? 60000 : 30000);
const jobName = (b: Build) =>
  conn(b.connectionId)?.targets?.find((t) => t.id === b.job)?.name || b.job;
const name = (id: string) => conn(id)?.name || `보존 연결 ${id.slice(0, 8)}`;
const selected = (id: string, targetID: string) =>
  scope === "*" ||
  (conn(id)?.targets || []).some(
    (t) =>
      t.id === targetID &&
      `${t.environment || "default"} / ${t.service || t.name}` === scope,
  );
const periods = () =>
  [900, 3600, 21600, 86400]
    .map(
      (p, i) =>
        `<option value="${p}" ${period === p ? "selected" : ""}>${["최근 15분", "최근 1시간", "최근 6시간", "최근 24시간"][i]}</option>`,
    )
    .join("");
function toast(message: unknown) {
  const el = document.querySelector("#toast")!;
  el.textContent = String(message);
  setTimeout(() => {
    if (el.textContent === String(message)) el.textContent = "";
  }, 6500);
}
function dispose() {
  charts.forEach((c) => c.dispose());
  charts = [];
}
function chart(el: HTMLElement, result: QueryResult, big = false) {
  if (!result.series?.some((s) => s.points?.some((p) => p.value !== null))) {
    el.innerHTML = '<div class="empty">표시할 숫자 데이터가 없습니다.</div>';
    return;
  }
  const c = echarts.init(el);
  charts.push(c);
  c.setOption({
    animation: false,
    color: ["#73e0db", "#9ebcff", "#f8cb7c", "#ffabb8", "#afc996"],
    grid: {
      left: big ? 55 : 40,
      right: 14,
      top: big ? 36 : 14,
      bottom: big ? 65 : 25,
    },
    tooltip: { trigger: "axis", renderMode: "richText", confine: true },
    legend: big
      ? { type: "scroll", textStyle: { color: "#a2b2c9" }, top: 0 }
      : undefined,
    xAxis: {
      type: "time",
      axisLabel: { color: "#a2b2c9", fontSize: 10, hideOverlap: true },
      axisLine: { lineStyle: { color: "#35445b" } },
      splitLine: { show: false },
    },
    yAxis: {
      type: "value",
      axisLabel: { color: "#a2b2c9", fontSize: 10 },
      splitLine: { lineStyle: { color: "#26364a" } },
    },
    dataZoom: big
      ? [
          { type: "inside" },
          {
            type: "slider",
            height: 16,
            bottom: 8,
            borderColor: "#334154",
            textStyle: { color: "#a2b2c9" },
          },
        ]
      : [],
    series: result.series.map((s) => ({
      name:
        Object.entries(s.labels || {})
          .map(([k, v]) => `${k}=${v}`)
          .join(", ") || "value",
      type: "line",
      showSymbol: s.points.length === 1,
      symbolSize: 5,
      connectNulls: false,
      lineStyle: { width: 2 },
      areaStyle: big ? undefined : { opacity: 0.07 },
      data: s.points.map((p) => [p.time * 1000, p.value]),
    })),
  });
}
new ResizeObserver(() => charts.forEach((c) => c.resize())).observe(
  document.body,
);

document.querySelector("#app")!.innerHTML =
  `<div class="app-shell"><header class="top"><div class="brand"><div class="logo" aria-hidden="true">≋</div><div><div class="eyebrow">DEVELOPER OPERATIONS</div><h1>IDP Dashboard</h1></div></div><div class="actions"><span class="muted" id="connection-count"></span><button id="refresh" class="ghost">↻ 새로고침</button></div></header><nav class="nav" aria-label="주 메뉴">${[
    ["overview", "운영 현황"],
    ["history", "빌드 이력"],
    ["metrics", "지표 탐색"],
    ["connections", "연결 설정"],
  ]
    .map(([id, label]) => `<button data-page="${id}">${label}</button>`)
    .join(
      "",
    )}</nav><div id="banner"></div><main id="page"></main><footer class="footer"><span>LOCAL FIRST · READ ONLY</span><span>앱 실행 중 수집 · 빌드 요약은 이 PC에 보존</span></footer></div><div id="toast" class="toast" role="status" aria-live="polite"></div><dialog id="connection-dialog" class="dialog" aria-labelledby="dialog-title"><form id="connection-form"><header class="dialog-header"><h2 id="dialog-title">연결 추가</h2><button type="button" data-close aria-label="닫기">×</button></header><div class="dialog-body" id="connection-fields"></div><footer class="dialog-footer"><button type="button" id="test-connection">연결 검사 · 대상 찾기</button><button type="submit" class="primary">저장</button></footer></form></dialog>`;

async function changePage(next: string) {
  page = next;
  dispose();
  generation++;
  renderedStamp = "";
  document
    .querySelectorAll("[data-page]")
    .forEach((el) =>
      el.setAttribute(
        "aria-current",
        el.getAttribute("data-page") === page ? "page" : "false",
      ),
    );
  await api
    .SavePreference("view", JSON.stringify({ page, scope, period }))
    .catch(() => {});
  if (page === "overview") renderOverview();
  if (page === "history") await renderHistory();
  if (page === "metrics") renderMetrics();
  if (page === "connections") renderConnections();
}
function updateChrome() {
  document.querySelector("#connection-count")!.textContent =
    `${state.connections.length}개 연결`;
  document.querySelector("#banner")!.innerHTML =
    (state.demo
      ? `<div class="notice info">${browserPreview ? "브라우저 미리보기" : "격리된 데모 모드"} · 예시 데이터입니다. 실제 인프라와 연결되지 않습니다.</div>`
      : "") +
    (state.error ? `<div class="notice error">${esc(state.error)}</div>` : "");
}
async function poll() {
  if (pollBusy) return;
  pollBusy = true;
  try {
    state = await api.GetState();
    updateChrome();
    const stamp = JSON.stringify([
      state.snapshots,
      state.connections,
      state.snapshots.map(fresh),
    ]);
    if (
      page === "overview" &&
      stamp !== renderedStamp &&
      !(document.querySelector("#connection-dialog") as HTMLDialogElement).open
    ) {
      renderedStamp = stamp;
      renderOverview();
    }
  } catch (e) {
    toast(`앱 상태 조회 실패: ${e}`);
  } finally {
    pollBusy = false;
  }
}

function renderOverview() {
  const root = document.querySelector("#page")!;
  const focused = root.contains(document.activeElement)
    ? (document.activeElement as HTMLElement)
    : null;
  const focusID = focused?.id;
  const detailID = focused?.closest("details")?.id;
  const scrollY = window.scrollY;
  const open = new Set(
    Array.from(root.querySelectorAll("details[open]")).map((d) => d.id),
  );
  dispose();
  const gen = ++generation;
  if (!state.connections.length) {
    root.innerHTML =
      '<section class="panel empty"><div class="empty-icon">◎</div><h2>첫 번째 도구를 연결하세요</h2><p>Jenkins, Argo CD, Prometheus의 주소를 등록하면 이곳에서 빌드·배포·지표를 함께 확인할 수 있습니다.</p><button class="primary" data-add>연결 추가</button></section>';
    return;
  }
  const scopes = [
    ...new Set(
      state.connections.flatMap((c) =>
        (c.targets || []).map(
          (t) => `${t.environment || "default"} / ${t.service || t.name}`,
        ),
      ),
    ),
  ].sort();
  if (scope !== "*" && !scopes.includes(scope)) scope = "*";
  const builds = state.snapshots
    .flatMap((s) =>
      (s.builds || []).filter((b) => selected(s.connectionId, b.job)),
    )
    .sort((a, b) => b.started - a.started);
  const latest = new Map<string, Build>();
  for (const b of builds) {
    const key = b.connectionId + b.job;
    if (b.status !== "RUNNING" && !latest.has(key)) latest.set(key, b);
  }
  const failed = [...latest.values()].filter(
    (b) => b.status === "FAILURE",
  ).length;
  const queues = state.snapshots.flatMap((s) =>
    (s.queue || [])
      .filter((q) => selected(s.connectionId, q.job))
      .map((q) => ({ ...q, connectionId: s.connectionId })),
  );
  const deploys = state.snapshots.flatMap((s) =>
    (s.deployments || [])
      .filter((d) => selected(s.connectionId, d.id))
      .map((d) => ({ ...d, connectionId: s.connectionId })),
  );
  const rules = state.snapshots.flatMap((s) =>
    (s.rules || []).map((r) => ({ ...r, connectionId: s.connectionId })),
  );
  const ruleValid = (r: (typeof rules)[number]) =>
    !r.error &&
    fresh(snap(r.connectionId)) &&
    r.result?.series?.some((s) =>
      s.points?.some(
        (p) => p.value !== null && Date.now() - p.time * 1000 < 120000,
      ),
    );
  const validRules = rules.filter(ruleValid);
  const badDeploy = deploys.filter(
    (d) => d.sync !== "Synced" || d.health !== "Healthy",
  ).length;
  const partial = state.connections.filter((c) => !fresh(snap(c.id)));
  const hasCI = state.connections.some(
    (c) =>
      supports(c, "builds") &&
      snap(c.id)?.lastSuccess &&
      new Date(snap(c.id)!.lastSuccess).getFullYear() > 2000,
  );
  const hasCD = state.connections.some(
    (c) =>
      supports(c, "deployments") &&
      snap(c.id)?.lastSuccess &&
      new Date(snap(c.id)!.lastSuccess).getFullYear() > 2000,
  );
  const stat = (
    label: string,
    value: number | string,
    hint: string,
    color = "",
  ) =>
    `<div class="stat"><div class="label">${label}</div><div class="value ${color}">${value}</div><div class="hint">${hint}</div></div>`;
  root.innerHTML = `<div class="page-head"><div><h2>운영 현황</h2><p>빌드, 배포, 관측 상태를 각 원본의 기준으로 확인합니다.</p></div><div class="toolbar"><select id="scope" aria-label="서비스 범위"><option value="*">모든 서비스</option>${scopes.map((s) => `<option ${s === scope ? "selected" : ""}>${esc(s)}</option>`).join("")}</select><select id="period" aria-label="조회 기간">${periods()}</select></div></div><div class="stats">${stat("실패한 빌드", hasCI ? failed : "—", "작업별 최신 완료 빌드", failed ? "bad" : "")}${stat("대기 중인 작업", hasCI ? queues.length : "—", "선택한 작업의 현재 queue")}${stat("배포 확인 필요", hasCD ? badDeploy : "—", "Sync · Health 별도 확인", badDeploy ? "warn" : "")}${stat("지표 이상 징후", validRules.length ? validRules.reduce((n, r) => n + r.breaches, 0) : "—", `판정 가능 ${validRules.length}/${rules.length}개 규칙 · 초과 시계열`, "warn")}</div>${partial.length ? `<div class="notice">${partial.map((c) => `${esc(c.name)}: ${esc(snap(c.id)?.error || "수집 대기 또는 조회 지연")}`).join(" · ")}<br><small>표시된 이전 값의 시각을 확인하세요. 조회 실패는 서비스 장애나 정상 상태를 뜻하지 않습니다.</small></div>` : ""}${state.snapshots
    .filter((s) => s.storageError)
    .map(
      (s) =>
        `<div class="notice error">${esc(name(s.connectionId))} · ${esc(s.storageError)}</div>`,
    )
    .join(
      "",
    )}<div class="grid"><section class="panel"><header class="panel-head"><h2>리소스 추이</h2><small>연결 전체 · 서비스 필터 미적용</small></header><div class="chart-grid">${
    rules
      .slice(0, 2)
      .map((r, i) => {
        const value = r.result?.series?.[0]?.points?.at(-1)?.value;
        return `<div><div class="metric-title">${esc(r.rule.name)} · ${esc(name(r.connectionId))}</div><div class="metric-value">${value == null ? "—" : value.toFixed(1)} <small>${esc(r.rule.unit)}</small></div><div id="overview-chart-${i}" class="chart"></div><div id="overview-note-${i}" class="chart-note">${r.error ? esc(r.error) : "추이 조회 중"}</div></div>`;
      })
      .join("") ||
    '<div class="empty span2">연결 설정에서 지표 규칙을 활성화하세요.</div>'
  }</div></section><section class="panel"><header class="panel-head"><h2>작업 흐름</h2><small>실행 ${builds.filter((b) => b.status === "RUNNING").length} · 대기 ${queues.length}</small></header><div class="list">${queues
    .slice(0, 2)
    .map(
      (q) =>
        `<div class="row"><div class="left"><strong>${esc(conn(q.connectionId)?.targets?.find((t) => t.id === q.job)?.name || q.job)}</strong><small>${esc(q.reason || "대기 사유 정보 없음")}</small></div><div class="right">${badge("QUEUED")}<small>${elapsed(Date.now() - q.since)}</small></div></div>`,
    )
    .join("")}${
    builds
      .slice(0, 4)
      .map(
        (b) =>
          `<div class="row"><div class="left"><strong>${esc(jobName(b))} <small style="display:inline">#${b.number}</small></strong><small>${esc(name(b.connectionId))} · ${date(b.started)}</small></div><div class="right">${badge(b.status)}<small>${elapsed(b.status === "RUNNING" ? Date.now() - b.started : b.duration)}</small></div></div>`,
      )
      .join("") ||
    (!queues.length ? '<div class="empty">수집된 빌드가 없습니다.</div>' : "")
  }</div></section></div><section class="panel services"><header class="panel-head"><h2>서비스 · 배포 상태</h2><small>${deploys.length}개 Application</small></header>${deploys.map((d, i) => `<details id="deploy-${i}" ${open.has(`deploy-${i}`) ? "open" : ""}><summary><strong>${esc(d.name)}<small style="display:block">${esc(name(d.connectionId))} · ${esc(d.project)}</small></strong><span>${badge(d.sync)}</span><span>${badge(d.health)}</span><span class="muted">⌄</span></summary><div class="detail"><p>배포 revision: <code>${esc((d.revisions || []).join(", ") || "정보 없음")}</code></p><p>최근 동기화 작업: ${esc(d.phase || "정보 없음")} · ${date(d.finished)}</p>${d.message ? `<p>${esc(d.message)}</p>` : ""}<button class="text-button" data-link="${esc(d.url)}">원본 Application 열기 ↗</button></div></details>`).join("") || '<div class="empty">연결 설정에서 조회할 Application을 선택하세요.</div>'}</section>${
    rules.length
      ? `<section class="panel" style="margin-top:20px"><header class="panel-head"><h2>이상 징후 규칙</h2><small>조건·원본 시각으로 판정</small></header><div class="list">${rules
          .map((r) => {
            const pts = r.result?.series?.flatMap((s) => s.points) || [];
            const valid = ruleValid(r);
            return `<div class="row"><div class="left"><strong>${esc(r.rule.name)}</strong><small>${esc(name(r.connectionId))} · ${esc(r.rule.description)} · 조건 &gt; ${r.rule.threshold} ${esc(r.rule.unit)}</small>${r.error ? `<small class="bad">${esc(r.error)}</small>` : ""}</div><div class="right">${badge(r.error || !valid ? "판정 불가" : r.breaches ? `${r.breaches}개 초과` : "정상")}<small>${pts.length ? date(pts[0].time * 1000) : "샘플 없음"}</small></div></div>`;
          })
          .join("")}</div></section>`
      : ""
  }<div class="footer">${state.connections.map((c) => `<span><i class="dot ${fresh(snap(c.id)) ? "good" : "warn"}"></i>${esc(c.name)} · ${date(snap(c.id)?.lastSuccess || "")}${snap(c.id)?.backfillPending ? " · 과거 기록 수집 중" : ""}</span>`).join("")}</div>`;
  if (focusID) document.getElementById(focusID)?.focus({ preventScroll: true });
  else if (detailID)
    document
      .getElementById(detailID)
      ?.querySelector("summary")
      ?.focus({ preventScroll: true });
  window.scrollTo(0, scrollY);
  rules.slice(0, 2).forEach(async (r, i) => {
    const key = `${r.connectionId}:${r.rule.expression}:${period}`;
    const cached = chartCache.get(key);
    try {
      let result = cached?.result;
      if (!cached || Date.now() - cached.at > 30000) {
        const end = Math.floor(Date.now() / 1000);
        result = await api.Query(r.connectionId, {
          expression: r.rule.expression,
          start: end - period,
          end,
        });
        chartCache.set(key, { result, at: Date.now() });
      }
      if (gen !== generation || page !== "overview") return;
      chart(
        document.querySelector(`#overview-chart-${i}`) as HTMLElement,
        result!,
      );
      document.querySelector(`#overview-note-${i}`)!.textContent =
        (result!.warnings || []).join(" · ") ||
        `${result!.series?.length || 0}개 시계열 · 최근 ${period / 60}분`;
    } catch (e) {
      if (gen === generation) {
        const el = document.querySelector(`#overview-note-${i}`);
        if (el) el.textContent = `추이 조회 실패: ${e}`;
      }
    }
  });
}

function buildRows(builds: Build[]) {
  return builds
    .map(
      (b) =>
        `<tr><td><strong>${esc(jobName(b))}</strong><br><small>${esc(name(b.connectionId))}</small></td><td>#${b.number}</td><td>${badge(b.status)}</td><td class="nowrap">${date(b.started)}</td><td>${elapsed(b.status === "RUNNING" ? Date.now() - b.started : b.duration)}</td><td><code>${esc(b.commit?.slice(0, 12) || "—")}</code></td><td><button class="text-button" data-link="${esc(b.url)}">원본 ↗</button></td></tr>`,
    )
    .join("");
}
async function renderHistory() {
  document.querySelector("#page")!.innerHTML =
    `<div class="page-head"><div><h2>빌드 이력</h2><p>이 PC가 관측한 빌드 요약입니다. 원본이 없어도 기록은 유지됩니다.</p></div><button id="backup">백업 내보내기</button></div><form id="history-filter" class="filterbar"><label>연결<select name="connectionId"><option value="">보존된 모든 연결</option>${state.connections
      .filter((c) => supports(c, "builds"))
      .map((c) => `<option value="${esc(c.id)}">${esc(c.name)}</option>`)
      .join(
        "",
      )}</select></label><label>작업 경로<input name="job" placeholder="작업 이름 또는 경로"></label><label>결과<select name="status"><option value="">모든 결과</option>${["SUCCESS", "FAILURE", "UNSTABLE", "ABORTED", "NOT_BUILT", "RUNNING", "UNCONFIRMED", "UNKNOWN"].map((s) => `<option>${s}</option>`).join("")}</select></label><label>시작일<input name="since" type="date"></label><label>종료일<input name="until" type="date"></label><button type="submit" style="align-self:end">조회</button></form><section class="panel"><div id="history-result" class="loading">로컬 이력 조회 중…</div></section><p class="muted" style="margin-top:14px;font-size:12px">자동 삭제하지 않습니다. 앱이 꺼진 동안 원본에서 삭제된 기록은 복구할 수 없습니다. RUNNING은 마지막 관측 상태이며 현재 실행 여부를 보장하지 않습니다.</p>`;
  await loadHistory();
}
async function loadHistory() {
  const form = document.querySelector("#history-filter") as HTMLFormElement;
  if (!form) return;
  const data = new FormData(form);
  const v = (key: string) => String(data.get(key) || "");
  try {
    const p = await api.History({
      connectionId: v("connectionId"),
      job: v("job"),
      status: v("status"),
      since: v("since") ? new Date(v("since") + "T00:00:00").getTime() : 0,
      until: v("until") ? new Date(v("until") + "T23:59:59.999").getTime() : 0,
      offset: historyOffset,
    });
    if (page !== "history") return;
    document.querySelector("#history-result")!.className = "";
    document.querySelector("#history-result")!.innerHTML =
      `<div class="table-wrap"><table><thead><tr><th>작업</th><th>빌드</th><th>결과</th><th>시작 시각</th><th>소요·경과</th><th>커밋</th><th>상세</th></tr></thead><tbody>${buildRows(p.builds)}</tbody></table>${!p.builds.length ? '<div class="empty">일치하는 보존 기록이 없습니다.</div>' : ""}</div><div class="pager"><small>${p.total}건 · DB ${(p.size / 1024 / 1024).toFixed(2)} MiB · ${historyOffset + 1}–${Math.min(historyOffset + 100, p.total)}</small><div class="toolbar"><button id="history-prev" ${historyOffset === 0 ? "disabled" : ""}>이전</button><button id="history-next" ${historyOffset + 100 >= p.total ? "disabled" : ""}>다음</button></div></div>`;
  } catch (e) {
    toast(e);
  }
}

function renderMetrics() {
  const metrics = state.connections.filter((c) => supports(c, "promql"));
  if (!metrics.some((c) => c.id === queryConnection))
    queryConnection = metrics[0]?.id || "";
  document.querySelector("#page")!.innerHTML =
    `<div class="page-head"><div><h2>지표 탐색</h2><p>PromQL을 직접 실행합니다. 서비스 필터는 자동 적용되지 않습니다.</p></div><span class="chip">READ ONLY · 최대 24시간</span></div>${metrics.length ? `<section class="panel"><div class="panel-body query-editor"><div class="toolbar"><select id="query-connection" aria-label="Prometheus 연결">${metrics.map((c) => `<option value="${esc(c.id)}" ${c.id === queryConnection ? "selected" : ""}>${esc(c.name)}</option>`).join("")}</select><select id="query-period" aria-label="쿼리 조회 기간">${periods()}</select><select id="query-mode" aria-label="조회 방식"><option value="range">시계열 조회</option><option value="instant">현재값 조회</option></select><select id="favorite" aria-label="즐겨찾기"><option value="">즐겨찾기</option>${favorites.map((f, i) => `<option value="${i}">${esc(f.name)}</option>`).join("")}</select></div><label class="sr-only" for="query-expression">PromQL</label><textarea id="query-expression" spellcheck="false">${esc(queryExpression)}</textarea><div class="toolbar"><button id="run-query" class="primary">쿼리 실행</button><button id="save-query">즐겨찾기 저장</button><small>5초 실행 제한 · 최대 100개 시계열 · 약 600개 점/시계열</small></div></div></section><section id="query-result" class="results"></section>` : '<section class="panel empty"><h2>Prometheus 연결이 필요합니다</h2><p>연결 설정에서 주소와 인증정보를 등록하세요.</p><button data-add class="primary">연결 추가</button></section>'}`;
}
async function runQuery() {
  const btn = document.querySelector("#run-query") as HTMLButtonElement;
  btn.disabled = true;
  const gen = generation;
  queryExpression = (
    document.querySelector("#query-expression") as HTMLTextAreaElement
  ).value;
  queryConnection = (
    document.querySelector("#query-connection") as HTMLSelectElement
  ).value;
  period = Number(
    (document.querySelector("#query-period") as HTMLSelectElement).value,
  );
  const end = Math.floor(Date.now() / 1000),
    instant =
      (document.querySelector("#query-mode") as HTMLSelectElement).value ===
      "instant";
  try {
    const r = await api.Query(queryConnection, {
      expression: queryExpression,
      start: instant ? 0 : end - period,
      end,
    });
    if (gen !== generation || page !== "metrics") return;
    dispose();
    document.querySelector("#query-result")!.innerHTML =
      `${r.warnings?.map((w) => `<div class="notice">${esc(w)}</div>`).join("") || ""}<div class="panel"><header class="panel-head"><h2>조회 결과</h2><small>${r.series?.length || 0}개 시계열 · ${date(Date.now())}</small></header><div class="panel-body"><div id="query-chart" class="chart big-chart"></div></div><div class="table-wrap"><table><thead><tr><th>라벨</th><th>마지막 값</th><th>샘플 시각</th></tr></thead><tbody>${(
        r.series || []
      )
        .map((s) => {
          const p = s.points?.at(-1);
          return `<tr><td class="series-label"><code>${esc(JSON.stringify(s.labels))}</code></td><td>${p?.value == null ? "결측" : esc(p.value)}</td><td>${p ? date(p.time * 1000) : "—"}</td></tr>`;
        })
        .join("")}</tbody></table></div></div>`;
    chart(document.querySelector("#query-chart") as HTMLElement, r, true);
  } catch (e) {
    if (page === "metrics")
      document.querySelector("#query-result")!.innerHTML =
        `<div class="notice error">${esc(e)}</div>`;
  } finally {
    btn.disabled = false;
  }
}

function renderConnections() {
  document.querySelector("#page")!.innerHTML =
    `<div class="page-head"><div><h2>연결 설정</h2><p>도구마다 주소와 조회 권한을 등록합니다. 같은 종류를 여러 개 연결할 수 있습니다.</p></div><button data-add class="primary">＋ 연결 추가</button></div><div class="connection-grid">${state.connections.map((c) => `<section class="panel connection-card"><div class="panel-body"><span class="chip">${esc(c.kind.toUpperCase())}</span><h3 style="margin-top:12px">${esc(c.name)}</h3><p class="address">${esc(c.url)}</p><p><i class="dot ${fresh(snap(c.id)) ? "good" : "warn"}"></i>${esc(snap(c.id)?.error || (fresh(snap(c.id)) ? "연결됨" : "수집 대기 · 지연"))}</p><small>마지막 성공 ${date(snap(c.id)?.lastSuccess || "")}<br>${supports(c, "promql") ? `${(c.rules || []).filter((r) => r.enabled).length}개 규칙` : `${(c.targets || []).length}개 감시 대상`} · ${esc(c.auth)} 인증${c.hasSecret ? " · 자격 증명 저장됨" : ""}</small><div class="toolbar"><button data-edit="${esc(c.id)}">설정 편집</button><button class="ghost danger" data-delete="${esc(c.id)}">연결 삭제</button></div></div></section>`).join("") || '<section class="panel empty"><h2>등록된 연결이 없습니다.</h2><p>Jenkins, Argo CD, Prometheus 중 필요한 도구부터 추가하세요.</p></section>'}</div><div class="notice info" style="margin-top:20px">토큰은 Windows 자격 증명 저장소에 보관합니다. 연결을 삭제해도 수집한 빌드 이력은 남습니다. API 주소를 바꿀 때는 이력 혼합을 방지하기 위해 새 연결을 만드세요.</div>`;
}
const blankConnection = (): Connection => ({
  id: "",
  kind: "jenkins",
  name: "",
  url: "",
  browserUrl: "",
  auth: "basic",
  username: "",
  caFile: "",
  targets: [],
  rules: [],
});
async function editConnection(id?: string) {
  editing = structuredClone(id ? conn(id)! : blankConnection());
  discovered = structuredClone(editing.targets || []);
  if (!editing.rules?.length) editing.rules = await api.Presets();
  document.querySelector("#dialog-title")!.textContent = id
    ? "연결 편집"
    : "연결 추가";
  document.querySelector("#connection-fields")!.innerHTML =
    `<div class="form-grid"><label>도구<select name="kind" ${id ? "disabled" : ""}>${state.providers
      .map(
        (p) =>
          `<option value="${esc(p.kind)}" ${editing!.kind === p.kind ? "selected" : ""}>${esc(p.name)} · ${esc(p.category)}</option>`,
      )
      .join(
        "",
      )}</select></label><label>연결 이름<input name="name" required value="${esc(editing.name)}" placeholder="예: Production CI"></label><label class="span2">API 기본 주소<input name="url" type="url" required ${id ? "readonly" : ""} value="${esc(editing.url)}" placeholder="https://ci.example.com/jenkins"></label><label>인증<select name="auth">${[
      ["none", "인증 없음"],
      ["basic", "Basic · 사용자명 + 토큰"],
      ["bearer", "Bearer 토큰"],
    ]
      .map(
        ([v, l]) =>
          `<option value="${v}" ${editing!.auth === v ? "selected" : ""}>${l}</option>`,
      )
      .join(
        "",
      )}</select></label><label>사용자명<input name="username" value="${esc(editing.username)}" autocomplete="off"></label><label class="span2">API 토큰 또는 비밀번호<input name="secret" type="password" autocomplete="new-password" placeholder="${editing.hasSecret ? "저장된 값 유지 · 교체할 때만 입력" : "조회용 자격증명 입력"}"></label><label>원본 화면 주소 · 선택<input name="browserUrl" type="url" value="${esc(editing.browserUrl)}" placeholder="API 주소와 다를 때"></label><label>사설 CA 파일 · 선택<div class="toolbar"><input name="caFile" style="flex:1" value="${esc(editing.caFile)}" placeholder="PEM 파일 경로"><button type="button" id="pick-ca">선택</button></div></label></div><div id="test-result" role="status" style="margin-top:15px"></div><div id="targets" class="targets"></div><div id="rules"></div>`;
  renderTargets();
  renderRules();
  (
    document.querySelector("#connection-dialog") as HTMLDialogElement
  ).showModal();
}
function renderTargets() {
  const el = document.querySelector("#targets")!;
  if (supports(editing, "promql")) {
    el.innerHTML = "";
    return;
  }
  el.innerHTML = `<div class="subheading"><h3>감시 대상</h3><small>연결 검사 후 선택 · 서비스·환경은 선택 입력</small></div>${
    discovered.length
      ? discovered
          .map((t, i) => {
            const saved = editing?.targets?.find((x) => x.id === t.id);
            return `<div class="target-row"><input type="checkbox" name="target-${i}" aria-label="${esc(t.name)} 감시" ${saved ? "checked" : ""}><div><strong>${esc(t.name)}</strong><small style="display:block;overflow-wrap:anywhere">${esc(t.id)}</small></div><input name="service-${i}" aria-label="${esc(t.name)} 서비스" placeholder="서비스" value="${esc(saved?.service || "")}"><input name="environment-${i}" aria-label="${esc(t.name)} 환경" placeholder="환경" value="${esc(saved?.environment || "")}"></div>`;
          })
          .join("")
      : '<p class="muted">연결 검사를 실행하면 조회 가능한 대상이 나타납니다.</p>'
  }`;
}
function renderRules() {
  const el = document.querySelector("#rules")!;
  if (!editing || !supports(editing, "promql")) {
    el.innerHTML = "";
    return;
  }
  el.innerHTML = `<div class="subheading"><h3>지표 규칙</h3><button type="button" id="add-rule">규칙 추가</button></div><p class="muted">필요한 메트릭이 수집되는 규칙만 활성화하세요. 조회 결과가 없으면 판정 불가로 표시합니다.</p>${(editing.rules || []).map((r, i) => `<div class="rule-row"><header><input type="checkbox" name="rule-enabled-${i}" aria-label="${esc(r.name)} 활성화" ${r.enabled ? "checked" : ""}><strong>${esc(r.name)}</strong></header><label class="sr-only" for="rule-expression-${i}">${esc(r.name)} PromQL</label><textarea id="rule-expression-${i}" name="rule-expression-${i}" spellcheck="false">${esc(r.expression)}</textarea><div class="rule-fields"><input name="rule-name-${i}" aria-label="규칙 이름" value="${esc(r.name)}"><input name="rule-threshold-${i}" aria-label="초과 임계값" type="number" step="any" value="${r.threshold}"><input name="rule-unit-${i}" aria-label="단위" value="${esc(r.unit)}" placeholder="단위"></div><input name="rule-description-${i}" aria-label="설명" style="width:100%" value="${esc(r.description)}"></div>`).join("")}`;
}
function readConnection() {
  const f = new FormData(
      document.querySelector("#connection-form") as HTMLFormElement,
    ),
    v = (k: string) => String(f.get(k) || "");
  const c = { ...editing! };
  for (const k of [
    "name",
    "url",
    "browserUrl",
    "auth",
    "username",
    "caFile",
  ] as const)
    c[k] = v(k).trim();
  c.kind = v("kind") || c.kind;
  c.targets = supports(c, "promql")
    ? []
    : discovered.flatMap((t, i) =>
        f.has(`target-${i}`)
          ? [
              {
                ...t,
                service: v(`service-${i}`).trim(),
                environment: v(`environment-${i}`).trim(),
              },
            ]
          : [],
      );
  c.rules = !supports(c, "promql")
    ? []
    : (editing?.rules || []).map((r, i) => ({
        ...r,
        enabled: f.has(`rule-enabled-${i}`),
        expression: v(`rule-expression-${i}`),
        name: v(`rule-name-${i}`),
        threshold: Number(v(`rule-threshold-${i}`)),
        unit: v(`rule-unit-${i}`),
        description: v(`rule-description-${i}`),
      }));
  return { connection: c, secret: v("secret") };
}

document.addEventListener("click", async (e) => {
  const button = (e.target as HTMLElement).closest("button");
  if (!button) return;
  try {
    if (button.dataset.page) {
      await changePage(button.dataset.page);
      return;
    }
    if (button.hasAttribute("data-add")) {
      await editConnection();
      return;
    }
    if (button.dataset.edit) {
      await editConnection(button.dataset.edit);
      return;
    }
    if (button.dataset.delete) {
      if (confirm("이 연결을 삭제할까요? 보존된 빌드 이력은 유지됩니다.")) {
        await api.DeleteConnection(button.dataset.delete);
        state = await api.GetState();
        updateChrome();
        renderConnections();
      }
      return;
    }
    if (button.dataset.link) {
      await api.OpenLink(button.dataset.link);
      return;
    }
    if (button.hasAttribute("data-close")) {
      (
        document.querySelector("#connection-dialog") as HTMLDialogElement
      ).close();
      return;
    }
    if (button.id === "refresh") {
      await api.Refresh();
      toast("새 조회를 요청했습니다.");
      await poll();
    }
    if (button.id === "backup") {
      const path = await api.Backup();
      if (path) toast(`백업 저장 완료\n${path}`);
    }
    if (button.id === "history-prev") {
      historyOffset = Math.max(0, historyOffset - 100);
      await loadHistory();
    }
    if (button.id === "history-next") {
      historyOffset += 100;
      await loadHistory();
    }
    if (button.id === "run-query") await runQuery();
    if (button.id === "save-query") {
      const expression = (
        document.querySelector("#query-expression") as HTMLTextAreaElement
      ).value.trim();
      if (!expression) return;
      favorites = favorites.filter((f) => f.expression !== expression);
      favorites.unshift({ name: expression.slice(0, 65), expression });
      favorites = favorites.slice(0, 30);
      await api.SavePreference("favorites", JSON.stringify(favorites));
      toast("즐겨찾기에 저장했습니다.");
      const select = document.querySelector("#favorite") as HTMLSelectElement;
      select.innerHTML =
        '<option value="">즐겨찾기</option>' +
        favorites
          .map((f, i) => `<option value="${i}">${esc(f.name)}</option>`)
          .join("");
    }
    if (button.id === "pick-ca") {
      const path = await api.PickCA();
      if (path)
        (document.querySelector("[name=caFile]") as HTMLInputElement).value =
          path;
    }
    if (button.id === "add-rule") {
      editing = readConnection().connection;
      editing.rules.push({
        id: crypto.randomUUID(),
        name: "Custom rule",
        expression: "up",
        threshold: 0,
        unit: "",
        description: "",
        enabled: false,
      });
      renderRules();
    }
    if (button.id === "test-connection") {
      const form = document.querySelector(
        "#connection-form",
      ) as HTMLFormElement;
      if (!form.reportValidity()) return;
      button.disabled = true;
      document.querySelector("#test-result")!.innerHTML =
        '<span class="muted">연결과 조회 권한을 확인하고 있습니다…</span>';
      try {
        const input = readConnection();
        discovered = await api.TestConnection(input);
        editing = input.connection;
        renderTargets();
        document.querySelector("#test-result")!.innerHTML =
          `<span class="good">연결 성공 · ${discovered.length}개 대상${supports(input.connection, "promql") ? " · 쿼리 API 확인" : ""}</span>`;
      } catch (err) {
        document.querySelector("#test-result")!.innerHTML =
          `<span class="bad">${esc(err)}</span>`;
      } finally {
        button.disabled = false;
      }
    }
  } catch (err) {
    toast(err);
  }
});
document.addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = e.target as HTMLFormElement;
  if (form.id === "history-filter") {
    historyOffset = 0;
    await loadHistory();
  }
  if (form.id === "connection-form") {
    const button = form.querySelector("[type=submit]") as HTMLButtonElement;
    button.disabled = true;
    try {
      await api.SaveConnection(readConnection());
      (
        document.querySelector("#connection-dialog") as HTMLDialogElement
      ).close();
      state = await api.GetState();
      updateChrome();
      await changePage("connections");
      toast("연결을 저장했습니다.");
    } catch (err) {
      toast(err);
    } finally {
      button.disabled = false;
    }
  }
});
document.addEventListener("change", async (e) => {
  const el = e.target as HTMLInputElement;
  if (el.id === "scope" || el.id === "period") {
    if (el.id === "scope") scope = el.value;
    else period = Number(el.value);
    await api.SavePreference("view", JSON.stringify({ page, scope, period }));
    renderOverview();
  }
  if (el.id === "favorite" && el.value !== "") {
    (document.querySelector("#query-expression") as HTMLTextAreaElement).value =
      favorites[Number(el.value)].expression;
  }
  if (el.name === "kind" && editing) {
    editing.kind = el.value;
    discovered = [];
    editing.targets = [];
    (document.querySelector("[name=auth]") as HTMLSelectElement).value =
      state.providers.find((p) => p.kind === el.value)?.defaultAuth || "none";
    renderTargets();
    renderRules();
  }
});
window.addEventListener("focus", () => {
  api.Refresh().catch(() => {});
  poll();
});
async function init() {
  try {
    const saved = await api.GetPreference("view");
    if (saved) {
      const v = JSON.parse(saved);
      if (["overview", "history", "metrics", "connections"].includes(v.page))
        page = v.page;
      if (typeof v.scope === "string") scope = v.scope;
      if ([900, 3600, 21600, 86400].includes(v.period)) period = v.period;
    }
    const f = await api.GetPreference("favorites");
    if (f) favorites = JSON.parse(f);
    state = await api.GetState();
    updateChrome();
    await changePage(page);
    setInterval(poll, 2000);
  } catch (e) {
    document.querySelector("#page")!.innerHTML =
      `<div class="notice error">앱을 초기화하지 못했습니다: ${esc(e)}</div>`;
  }
}
void init();
