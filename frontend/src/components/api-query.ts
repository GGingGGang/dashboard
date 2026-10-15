import { api } from "../bridge";
import { esc, date } from "../shared/format";
import { validateJSONRequest, type JSONRequest, type JSONResult } from "../shared/json-query";
import type { DashboardWidget } from "./dashboard-layout";

export function mountAPIQuery(root: HTMLElement, isCurrent: () => boolean,
  add: (request: JSONRequest, title: string, result: JSONResult, existing?: string) => Promise<void>, existing?: DashboardWidget) {
  const initial = existing?.json || { source: "http", url: "", filter: ".service.file.name" };
  root.innerHTML = `<section class="api-query"><header class="page-head"><div><h2>API 쿼리</h2><p>JSON 응답에서 필요한 값을 꺼내 노드에 표시합니다.</p></div></header>
    <form class="panel dashboard-widget-fields" id="api-query-form">
      <div class="form-grid"><label>데이터 가져오기<select name="source"><option value="http">HTTP API</option><option value="command" disabled>Command · 준비 중</option></select></label><label>노드 이름<input name="title" maxlength="80" required value="${esc(existing?.title || "API 결과")}"></label></div>
      <label>API URL · GET<input name="url" type="url" maxlength="8192" required placeholder="http://localhost:8080/status" value="${esc(initial.url)}"></label>
      <label>jq 조회식<textarea name="filter" rows="3" maxlength="16384" required spellcheck="false">${esc(initial.filter)}</textarea></label>
      <p class="muted">예: <code>.service.file.name</code> · 배열 합계: <code>[.items[].cost] | add</code> · 전체 응답: <code>.</code></p>
      <p class="muted">인증 없는 JSON API를 지원합니다. URL에 토큰을 넣지 마세요. 조회는 버튼을 누를 때만 실행합니다.</p>
      <div class="actions"><button class="primary" type="submit" id="api-run">조회 실행</button><button type="button" id="api-add" disabled>${existing ? "노드 설정 적용" : "대시보드에 추가"}</button></div>
      <div id="api-error" class="bad" role="alert"></div>
    </form><section class="panel api-result" aria-label="조회 결과"><p class="muted">URL과 jq 식을 입력하고 조회 실행을 누르세요.</p></section></section>`;
  const form = root.querySelector<HTMLFormElement>("form")!;
  const run = root.querySelector<HTMLButtonElement>("#api-run")!;
  const addButton = root.querySelector<HTMLButtonElement>("#api-add")!;
  const error = root.querySelector<HTMLElement>("#api-error")!;
  let successful: { request: JSONRequest; result: JSONResult } | null = null;
  let revision = 0;
  let busy = false;
  const capture = () => {
    const values = new FormData(form);
    return validateJSONRequest({ source: values.get("source"), url: String(values.get("url")).trim(), filter: String(values.get("filter")) });
  };
  form.oninput = event => {
    if ((event.target as HTMLInputElement).name === "title") return;
    revision++; successful = null; addButton.disabled = true;
    root.querySelector(".api-result")!.textContent = "설정이 바뀌었습니다. 다시 조회하세요.";
  };
  form.onsubmit = async event => {
    event.preventDefault();
    if (busy) return;
    error.textContent = "";
    successful = null;
    addButton.disabled = true;
    const started = revision;
    try {
      const request = capture();
      busy = true; run.disabled = true; run.textContent = "조회 중…";
      root.querySelector(".api-result")!.textContent = "조회 중…";
      const result = await api.ReadJSON(request);
      if (!isCurrent() || started !== revision) return;
      successful = { request, result };
      root.querySelector(".api-result")!.innerHTML = `<p class="db-meta">응답 ${esc(date(result.observedAt))} · ${result.count}개 결과</p>${result.scalar ? `<div class="db-current">${esc(result.text)}</div>` : `<pre>${esc(result.json)}</pre>`}<details><summary>조회 결과 JSON</summary><pre>${esc(result.json)}</pre></details>`;
      addButton.disabled = false;
    } catch (reason) { if (isCurrent() && started === revision) {
      error.textContent = String(reason);
      root.querySelector(".api-result")!.textContent = "조회하지 못했습니다. 주소와 jq 식을 확인하세요.";
    } }
    finally { busy = false; if (isCurrent()) { run.disabled = false; run.textContent = "조회 실행"; } }
  };
  addButton.onclick = async () => {
    if (!successful || !form.reportValidity()) return;
    addButton.disabled = true;
    try { await add(successful.request, String(new FormData(form).get("title")).trim() || "API 결과", successful.result, existing?.id); }
    catch (reason) { if (isCurrent()) { error.textContent = String(reason); addButton.disabled = false; } }
  };
}
