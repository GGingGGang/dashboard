import { api } from "../bridge";
import type { State, Observation, LogResult, TraceSearchResult, TraceResult } from "../shared/types";
import { esc, date } from "../shared/format";

const preciseTime = (ns: string) => {
  try { return `${date(Number(BigInt(ns) / 1000000n))} · ${ns} ns`; }
  catch { return ns || "시각 없음"; }
};
const observation = (o: Observation) => `<div class="notice ${o.outcome === "error" ? "error" : "info"}" role="status">${o.outcome === "error" ? "조회 실패" : o.completeness === "complete" ? "요청 범위 조회 완료" : "일부 결과 · 전체 데이터 여부 미확인"} · 관측 ${esc(date(o.observedAt))}${o.windowStart ? `<br>범위 ${esc(date(o.windowStart * 1000))} — ${esc(date(o.windowEnd * 1000))}` : ""}${(o.diagnostics || []).map(d => `<br>${esc(d)}`).join("")}</div>`;
const json = (value: unknown) => esc(JSON.stringify(value, null, 2));

function logs(r: LogResult) {
  return observation(r.observation) + `<div class="table-wrap"><table><thead><tr><th>시각</th><th>로그</th></tr></thead><tbody>${(r.entries || []).map(e => `<tr><td class="event-time">${esc(preciseTime(e.timestampNs))}</td><td><pre class="event-text">${esc(e.line)}</pre><details><summary>라벨 · 구조화 필드</summary><pre class="event-text">${json({ labels: e.labels, fields: e.fields })}</pre></details></td></tr>`).join("")}</tbody></table></div>${!r.entries?.length && r.observation.outcome !== "error" ? '<p class="empty">요청 범위의 결과가 없습니다.</p>' : ""}`;
}
function traces(r: TraceSearchResult) {
  return observation(r.observation) + `<div class="table-wrap"><table><thead><tr><th>Trace ID</th><th>루트 서비스 · 작업</th><th>시각 · 소요 시간</th></tr></thead><tbody>${(r.traces || []).map(t => `<tr><td><button class="trace-link" data-trace-id="${esc(t.traceId)}">${esc(t.traceId)}</button></td><td>${esc(t.rootService || "미제공")}<br>${esc(t.rootName || "미제공")}</td><td>${esc(preciseTime(t.startNs))}<br>${esc(t.durationMs)} ms</td></tr>`).join("")}</tbody></table></div>${!r.traces?.length && r.observation.outcome !== "error" ? '<p class="empty">요청 범위의 결과가 없습니다.</p>' : ""}`;
}
function trace(r: TraceResult) {
  const spans = r.spans || [];
  const ids = new Set(spans.map(s => s.spanId));
  return `<h3>트레이스 상세 · ${esc(r.traceId)}</h3>` + observation(r.observation) + spans.map(s => `<details class="span-detail"><summary>${esc(s.service || "서비스 미제공")} · ${esc(s.name)} · ${esc(s.durationNs)} ns</summary><dl><dt>Span ID</dt><dd>${esc(s.spanId)}</dd><dt>Parent span ID</dt><dd>${esc(s.parentSpanId || "미제공")}${s.parentSpanId && !ids.has(s.parentSpanId) ? " · 응답에 부모 span 없음" : ""}</dd><dt>시작</dt><dd>${esc(preciseTime(s.startNs))}</dd></dl><pre class="event-text">${json({ attributes: s.attributes, resource: s.resource, scope: s.scope, span: s.details })}</pre></details>`).join("") + (!spans.length && r.observation.outcome !== "error" ? '<p class="empty">반환된 span이 없습니다.</p>' : "");
}

export function mountExplorer(root: HTMLElement, kind: "logs" | "traces", state: State, active: () => boolean, version: (id: string) => string) {
  const connections = state.connections.filter(c => state.providers.find(p => p.kind === c.kind)?.capabilities.includes(kind));
  const title = kind === "logs" ? "로그 탐색" : "트레이스 탐색";
  root.innerHTML = `<div class="page-head"><div><h2>${title}</h2><p>선택한 연결에 직접 질의합니다. 서비스 필터는 자동 적용되지 않습니다.</p></div><span class="chip">READ ONLY · 최대 24시간</span></div>${connections.length ? `<section class="panel"><div class="panel-body query-editor"><div class="toolbar"><select id="explore-connection" aria-label="탐색 연결">${connections.map(c => `<option value="${esc(c.id)}">${esc(c.name)}</option>`).join("")}</select><select id="explore-period" aria-label="탐색 기간"><option value="900">최근 15분</option><option value="3600" selected>최근 1시간</option><option value="21600">최근 6시간</option><option value="86400">최근 24시간</option></select><label>최대 건수 <input id="explore-limit" type="number" min="1" value="100"></label></div><label for="explore-expression" id="explore-language"></label><textarea id="explore-expression" spellcheck="false"></textarea><button id="explore-run" class="primary">검색</button>${kind === "traces" ? '<div class="toolbar"><label for="trace-id">Trace ID</label><input id="trace-id" placeholder="16진수 Trace ID" maxlength="32"><button id="trace-open">ID로 상세 조회</button></div>' : ""}</div></section><section id="explore-result" class="panel results" aria-live="polite"></section><section id="trace-result" class="panel results" aria-live="polite"></section>` : `<section class="panel empty"><h2>${title} 연결이 필요합니다</h2><button data-add class="primary">연결 추가</button></section>`}`;
  if (!connections.length) return;
  const select = root.querySelector<HTMLSelectElement>("#explore-connection")!;
  const expression = root.querySelector<HTMLTextAreaElement>("#explore-expression")!;
  const limit = root.querySelector<HTMLInputElement>("#explore-limit")!;
  const period = root.querySelector<HTMLSelectElement>("#explore-period")!;
  const result = root.querySelector<HTMLElement>("#explore-result")!;
  const detail = root.querySelector<HTMLElement>("#trace-result")!;
  const run = root.querySelector<HTMLButtonElement>("#explore-run")!;
  let request = 0;
  let detailRequest = 0;
  const configure = () => {
    const c = connections.find(c => c.id === select.value)!;
    const meta = state.providers.find(p => p.kind === c.kind)?.queries?.[kind];
    root.querySelector("#explore-language")!.textContent = meta?.language || "Query";
    expression.value = meta?.defaultExpression || "";
    limit.max = String(meta?.maxResults || 100);
    limit.value = String(Math.min(Number(limit.value), Number(limit.max)));
  };
  const invalidate = () => { request++; detailRequest++; run.disabled = false; result.innerHTML = ""; detail.innerHTML = ""; };
  select.addEventListener("change", () => { invalidate(); configure(); });
  for (const control of [expression, limit, period]) control.addEventListener("input", invalidate);
  configure();
  run.addEventListener("click", async () => {
    if (!limit.reportValidity() || !expression.value.trim()) return;
    const token = ++request, id = select.value, revision = version(id);
    detailRequest++; detail.innerHTML = "";
    const current = () => active() && token === request && select.value === id && version(id) === revision;
    run.disabled = true; result.textContent = "조회 중…";
    const end = Math.floor(Date.now() / 1000);
    const q = { expression: expression.value, start: end - Number(period.value), end, limit: Number(limit.value) };
    try {
      const response = kind === "logs" ? await api.SearchLogs(id, q) : await api.SearchTraces(id, q);
      if (current()) result.innerHTML = kind === "logs" ? logs(response as LogResult) : traces(response as TraceSearchResult);
    } catch (e) { if (current()) result.innerHTML = `<div class="notice error">${esc(String(e))}</div>`; }
    finally { if (current()) run.disabled = false; }
  });
  const openTrace = async (traceID: string) => {
    const token = ++detailRequest, id = select.value, revision = version(id);
    const current = () => active() && token === detailRequest && select.value === id && version(id) === revision;
    detail.textContent = "트레이스 조회 중…";
    try { const r = await api.Trace(id, traceID); if (current()) detail.innerHTML = trace(r); }
    catch (e) { if (current()) detail.innerHTML = `<div class="notice error">${esc(String(e))}</div>`; }
  };
  root.querySelector("#trace-open")?.addEventListener("click", () => void openTrace(root.querySelector<HTMLInputElement>("#trace-id")!.value.trim()));
  root.querySelector("#trace-id")?.addEventListener("input", () => { detailRequest++; detail.innerHTML = ""; });
  result.addEventListener("click", e => { const id = (e.target as HTMLElement).closest<HTMLElement>("[data-trace-id]")?.dataset.traceId; if (id) void openTrace(id); });
}
