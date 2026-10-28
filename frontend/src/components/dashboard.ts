import type { ExtensionBinding } from "../bridge/extensions";
import { api } from "../bridge";
import type { Connection, State } from "../shared/types";
import type { JSONRequest, JSONResult } from "../shared/json-query";
import { esc } from "../shared/format";
import { DashboardData } from "./dashboard-data";
import { COLUMNS, LAYOUT_KEY, MAX_WIDGETS, defaultLayout, nextPosition, parseLayout, placeWidget } from "./dashboard-layout";
import type { DashboardLayout, DashboardWidget, WidgetKind } from "./dashboard-layout";
import "./dashboard.css";

const kinds: Record<WidgetKind, string> = {
  builds: "빌드", queue: "빌드 대기열", deployments: "배포 상태",
  metrics: "지표", logs: "로그", traces: "트레이스", json: "API 쿼리", extension: "플러그인",
};
const queryKinds = new Set<WidgetKind>(["metrics", "logs", "traces", "json", "extension"]);
const clone = <T>(value: T): T => structuredClone(value);
const bounded = (value: number, min: number, max: number) => Math.max(min, Math.min(value, max));
const ROW = 76, GAP = 12;
type Gesture = {
  widget: DashboardWidget; proposal: DashboardWidget; mode: "move" | "resize";
  x: number; y: number; pointer: number; handle: HTMLElement; ghost: HTMLElement;
};

export class DashboardController {
  private root: HTMLElement | null = null;
  private layout: DashboardLayout = { version: 2, widgets: [] };
  private baseline: DashboardLayout = clone(this.layout);
  private loading: Promise<void> | null = null;
  private loadError = "";
  private saveError = "";
  private editing = false;
  private saving = false;
  private undo: DashboardLayout[] = [];
  private data: DashboardData | null = null;
  private resizeObserver: ResizeObserver | null = null;
  private gesture: Gesture | null = null;
  private bodyMarkup = new Map<string, string>();
  private session = 0;
  private pendingJSON = new Map<string, JSONResult>();

  constructor(private getState: () => State, private report: (message: unknown) => void, private openAPI: (widget?: DashboardWidget) => void = () => {}, private openPlugins: (widget?: DashboardWidget) => void = () => {}) {}

  async addExtension(extension: ExtensionBinding, connectionId: string, title: string, id?: string) {
    this.loading ||= this.load(); await this.loading;
    if (this.loadError || this.saving) throw new Error(this.loadError || "배치를 저장 중입니다.");
    const old = id ? this.layout.widgets.find(w => w.id === id) : undefined;
    if (id && !old) throw new Error("수정할 카드가 없습니다.");
    const widget: DashboardWidget = old ? { ...old, extension, connectionId, title } : {
      id: crypto.randomUUID(), kind: "extension", title, extension, connectionId,
      targetId: "", expression: "", unit: "", period: 900, refreshSeconds: 60,
      ...nextPosition(this.layout.widgets, 6, 4), w: 6, h: 4,
    };
    this.editing = true; this.change(placeWidget(this.layout.widgets, widget));
  }

  async addJSON(request: JSONRequest, title: string, result: JSONResult, id?: string) {
    this.loading ||= this.load();
    await this.loading;
    if (this.loadError || this.saving) throw new Error(this.loadError || "배치를 저장 중입니다.");
    const old = id ? this.layout.widgets.find(widget => widget.id === id) : undefined;
    if (id && !old) throw new Error("수정할 노드가 없습니다.");
    const widget: DashboardWidget = old ? { ...old, title, json: request } : {
      id: crypto.randomUUID(), kind: "json", title, json: request,
      connectionId: "", targetId: "", expression: "", unit: "", period: 900, refreshSeconds: 0,
      ...nextPosition(this.layout.widgets, 4, 3), w: 4, h: 3,
    };
    const widgets = placeWidget(this.layout.widgets, widget);
    this.editing = true;
    this.change(widgets);
    this.pendingJSON.set(widget.id, result);
  }

  async mount(root: HTMLElement) {
    this.root = root;
    const session = ++this.session;
    root.innerHTML = '<div class="panel empty" role="status">내 대시보드를 불러오고 있습니다…</div>';
    this.loading ||= this.load();
    await this.loading;
    if (!this.root || session !== this.session) return;
    this.data = new DashboardData(this.getState, id => this.renderBody(id));
    this.data.update(this.layout.widgets);
    for (const [id, result] of this.pendingJSON) this.data.seedJSON(id, result);
    this.pendingJSON.clear();
    this.render();
    this.resizeObserver = new ResizeObserver(() => {
      if (this.gesture) this.cancelGesture();
      this.positionCards();
    });
    this.resizeObserver.observe(root);
  }

  private async load() {
    try {
      const raw = await api.GetPreference(LAYOUT_KEY);
      this.layout = parseLayout(raw) || defaultLayout(this.getState());
      if (raw && JSON.parse(raw).version === 1 && !await api.GetPreference(`${LAYOUT_KEY}.v1.backup`)) {
        await api.SavePreference(`${LAYOUT_KEY}.v1.backup`, raw);
      }
      this.baseline = clone(this.layout);
      this.loadError = "";
    } catch (error) {
      this.loadError = String(error);
    }
  }

  unmount() {
    this.cancelGesture();
    this.session++;
    this.resizeObserver?.disconnect();
    this.resizeObserver = null;
    this.data?.stop();
    this.data = null;
    this.root = null;
  }

  update() {
    if (!this.root || !this.data) return;
    this.data.update(this.layout.widgets);
    if (!this.gesture) this.layout.widgets.forEach(w => this.renderBody(w.id));
  }

  refresh() { this.data?.refresh(); }

  private board() { return this.root?.querySelector<HTMLElement>("#dashboard-board"); }
  private compact() { return (this.board()?.clientWidth || 0) < 680; }
  private dirty() { return JSON.stringify(this.layout) !== JSON.stringify(this.baseline); }
  private message(value: string) {
    const el = this.root?.querySelector<HTMLElement>("#dashboard-message");
    if (el) el.textContent = value;
  }

  private render() {
    if (!this.root) return;
    const dirty = this.dirty();
    this.bodyMarkup.clear();
    this.root.innerHTML = `<section class="dashboard-page">
      <header class="page-head dashboard-heading"><div><h2>내 대시보드</h2><p>보고 싶은 정보를, 편한 위치에.</p></div>
      <div class="actions">${this.editing ? `<button id="dashboard-undo" ${!this.undo.length || this.saving ? "disabled" : ""}>되돌리기</button><button id="dashboard-plugins">플러그인 정보 추가</button><button id="dashboard-add" ${this.layout.widgets.length >= MAX_WIDGETS || this.saving ? "disabled" : ""}>＋ 정보 추가</button><button id="dashboard-cancel" ${this.saving ? "disabled" : ""}>취소</button>` : ""}<button id="dashboard-edit" class="primary" ${this.loadError || this.saving ? "disabled" : ""}>${this.saving ? "저장 중…" : this.editing ? "저장 · 배치 완료" : "배치 편집"}</button></div></header>
      ${this.loadError ? `<div class="notice error" role="alert">저장된 배치를 열지 못했습니다. 기존 배치는 덮어쓰지 않았습니다.<br>${esc(this.loadError)}<button id="dashboard-reload">다시 불러오기</button></div>` : ""}
      ${this.saveError ? `<div class="notice error" role="alert">배치 저장 실패: ${esc(this.saveError)}<br>편집 내용은 이 창에 남아 있습니다. 저장을 다시 시도하세요.</div>` : ""}
      <div class="dashboard-status"><span>${this.editing ? "배치 편집 중" : "내 작업 공간"}</span><span id="dashboard-message" role="status" aria-live="polite">${this.editing ? dirty ? "저장하지 않은 변경사항" : "제목을 끌어 이동 · 모서리로 크기 조절" : `${this.layout.widgets.length}개 정보 · 배치 고정`}</span></div>
      ${this.editing ? '<p class="dashboard-narrow-hint muted">좁은 창에서는 세로로 표시합니다. 크기는 카드 설정에서 바꿀 수 있고, 가로 배치는 넓은 창에서 편집합니다.</p>' : ""}
      <div id="dashboard-board" class="dashboard-board ${this.editing ? "is-editing" : ""}" aria-label="대시보드 배치 영역"></div>
      <dialog id="dashboard-widget-dialog" class="dialog" aria-labelledby="dashboard-widget-title"><form id="dashboard-widget-form"></form></dialog>
    </section>`;
    this.root.querySelector<HTMLButtonElement>("#dashboard-plugins")?.addEventListener("click", () => this.openPlugins());
    const board = this.board()!;
    if (!this.loadError && !this.layout.widgets.length) {
      board.innerHTML = `<div class="dashboard-empty"><h3>${this.getState().connections.length ? "내 작업 공간을 구성해보세요" : "첫 번째 도구를 연결하세요"}</h3><p>${this.getState().connections.length ? "배치 편집에서 원하는 정보를 추가할 수 있습니다." : "도구를 연결한 뒤 빌드·배포·지표를 원하는 위치에 놓을 수 있습니다."}</p>${this.getState().connections.length ? "" : '<button class="primary" data-add>연결 추가</button>'}</div>`;
    }
    [...this.layout.widgets].sort((a, b) => a.y - b.y || a.x - b.x).forEach(w => {
      const card = document.createElement("section");
      card.className = `dashboard-widget ${w.h <= 2 ? "is-compact" : ""}`;
      card.dataset.widgetId = w.id;
      card.setAttribute("aria-label", w.title || kinds[w.kind]);
      card.innerHTML = `<header class="dashboard-widget-header">${this.editing ? '<button type="button" class="dashboard-move" data-widget-action="move" aria-label="위치 이동 · 방향키 사용 가능">⠿</button>' : ""}<h3 class="dashboard-widget-title">${esc(w.title || kinds[w.kind])}</h3><div class="dashboard-widget-actions">${this.editing ? '<button type="button" data-widget-action="settings" aria-label="정보 설정">설정</button><button type="button" data-widget-action="remove" aria-label="정보 제거">×</button>' : queryKinds.has(w.kind) ? '<button type="button" data-widget-action="refresh" aria-label="이 정보 새로고침">↻</button>' : ""}</div></header><div class="dashboard-widget-body"></div>${this.editing ? '<button class="dashboard-resize" type="button" data-widget-action="resize" aria-label="크기 조절 · 방향키 사용 가능">↘</button>' : ""}`;
      board.appendChild(card);
      card.addEventListener("wheel", event => {
        if (!event.ctrlKey || w.kind !== "metrics" || !event.deltaY || this.gesture) return;
        const graph = card.querySelector<SVGElement>(".db-chart");
        if (!graph) return;
        event.preventDefault();
        const bounds = graph.getBoundingClientRect();
        const delta = event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? 200 : 1);
        this.data?.zoomGraph(w.id, 2 ** (-bounded(delta, -200, 200) / 400),
          bounds.width ? (event.clientX - bounds.left) / bounds.width : 0.5);
      }, { passive: false });
      card.addEventListener("click", event => {
        const action = (event.target as HTMLElement).closest<HTMLButtonElement>("[data-graph-action]")?.dataset.graphAction;
        if (action) this.data?.zoomGraph(w.id, action === "reset" ? null : action === "in" ? 1.25 : 0.8);
      });
      card.querySelectorAll<HTMLButtonElement>("[data-widget-action]").forEach(button => {
        const action = button.dataset.widgetAction;
        if (action === "settings") button.onclick = () => this.openEditor(w);
        if (action === "remove") button.onclick = () => this.change(this.layout.widgets.filter(v => v.id !== w.id));
        if (action === "refresh") button.onclick = () => this.data?.refresh(w.id);
        if (action === "move" || action === "resize") button.onkeydown = event => this.moveKey(event, w, action);
      });
      if (this.editing) {
        card.querySelector<HTMLElement>(".dashboard-widget-header")!.onpointerdown = e => {
          if ((e.target as HTMLElement).closest('[data-widget-action="settings"],[data-widget-action="remove"]')) return;
          this.beginGesture(e, w, "move");
        };
        card.querySelector<HTMLElement>('[data-widget-action="resize"]')!.onpointerdown = e => this.beginGesture(e, w, "resize");
      }
      this.renderBody(w.id);
      this.data?.attach(w.id, card);
    });
    this.root.querySelector<HTMLButtonElement>("#dashboard-edit")!.onclick = () => {
      if (this.editing) void this.save();
      else { this.editing = true; this.undo = []; this.render(); this.root?.querySelector<HTMLButtonElement>("#dashboard-add")?.focus(); }
    };
    this.root.querySelector<HTMLButtonElement>("#dashboard-add")?.addEventListener("click", () => this.openEditor());
    this.root.querySelector<HTMLButtonElement>("#dashboard-cancel")?.addEventListener("click", () => {
      this.layout = clone(this.baseline); this.undo = []; this.editing = false; this.saveError = "";
      this.data?.update(this.layout.widgets); this.render();
    });
    this.root.querySelector<HTMLButtonElement>("#dashboard-undo")?.addEventListener("click", () => {
      const last = this.undo.pop();
      if (last) { this.layout = last; this.data?.update(this.layout.widgets); this.render(); this.message("이전 배치로 되돌렸습니다."); }
    });
    this.root.querySelector<HTMLButtonElement>("#dashboard-reload")?.addEventListener("click", async () => {
      await this.load();
      if (this.root) { this.data?.update(this.layout.widgets); this.render(); }
    });
    this.positionCards();
  }

  private renderBody(id: string) {
    if (!this.root || !this.data || this.gesture) return;
    const widget = this.layout.widgets.find(w => w.id === id);
    const card = [...this.root.querySelectorAll<HTMLElement>(".dashboard-widget")].find(el => el.dataset.widgetId === id);
    const body = card?.querySelector<HTMLElement>(".dashboard-widget-body");
    if (!widget || !body) return;
    const html = this.data.markup(widget);
    const focused = body.contains(document.activeElement);
    const graphAction = focused ? (document.activeElement as HTMLElement).dataset.graphAction : undefined;
    if (this.bodyMarkup.get(id) === html || (focused && !graphAction)) return;
    const scroll = body.scrollTop;
    body.innerHTML = html;
    body.scrollTop = scroll;
    this.bodyMarkup.set(id, html);
    if (graphAction) body.querySelector<HTMLButtonElement>(`[data-graph-action="${graphAction}"]`)?.focus({ preventScroll: true });
  }

  private position(el: HTMLElement, widget: DashboardWidget) {
    const unit = ((this.board()?.clientWidth || 0) + GAP) / COLUMNS;
    el.style.left = `${widget.x * unit}px`;
    el.style.top = `${widget.y * ROW}px`;
    el.style.width = `${widget.w * unit - GAP}px`;
    el.style.height = `${widget.h * ROW - GAP}px`;
  }

  private positionCards() {
    const board = this.board();
    if (!board) return;
    board.classList.toggle("is-narrow", this.compact());
    board.querySelectorAll<HTMLElement>(".dashboard-widget").forEach(el => {
      const widget = this.layout.widgets.find(w => w.id === el.dataset.widgetId);
      if (widget) this.position(el, widget);
    });
    board.style.height = `${Math.max(2, ...this.layout.widgets.map(w => w.y + w.h)) * ROW + (this.editing ? ROW : 0)}px`;
  }

  private change(widgets: DashboardWidget[]) {
    if (this.saving || !this.editing) return;
    this.undo.push(clone(this.layout));
    if (this.undo.length > 30) this.undo.shift();
    this.layout = { version: 2, widgets };
    this.saveError = "";
    this.data?.update(widgets);
    this.render();
  }

  private async save() {
    if (this.saving || this.loadError) return;
    this.saving = true;
    this.saveError = "";
    const payload = JSON.stringify(this.layout);
    this.render();
    try {
      parseLayout(payload);
      await api.SavePreference(LAYOUT_KEY, payload);
      this.baseline = clone(this.layout);
      this.editing = false;
      this.undo = [];
      this.report("대시보드 배치를 저장했습니다.");
    } catch (error) {
      this.saveError = String(error);
    } finally {
      this.saving = false;
      this.render();
    }
  }

  private compatible(kind: WidgetKind): Connection[] {
    const state = this.getState();
    return state.connections.filter(c => state.providers.find(p => p.kind === c.kind)?.capabilities.includes(kind));
  }

  private openEditor(existing?: DashboardWidget) {
    if (!this.root || this.saving) return;
    if (existing?.kind === "json") { this.openAPI(existing); return; }
    if (existing?.kind === "extension") { this.openPlugins(existing); return; }
    const available = (Object.keys(kinds) as WidgetKind[]).filter(k => k !== "extension" && (k === "json" || this.compatible(k).length));
    if (!available.length) { this.report("먼저 연결 설정에서 사용할 도구를 연결하세요."); return; }
    const kind = existing?.kind || available[0];
    if (kind === "json") { this.openAPI(); return; }
    const draft: DashboardWidget = existing ? clone(existing) : {
      id: crypto.randomUUID(), kind, title: kinds[kind], connectionId: this.compatible(kind)[0]?.id || "",
      targetId: "", expression: "", unit: "", period: 900,
      refreshSeconds: kind === "metrics" ? 60 : 0, ...nextPosition(this.layout.widgets, 4, queryKinds.has(kind) ? 4 : 3), w: 4, h: queryKinds.has(kind) ? 4 : 3,
    };
    const dialog = this.root.querySelector<HTMLDialogElement>("#dashboard-widget-dialog")!;
    const form = dialog.querySelector<HTMLFormElement>("form")!;
    const capture = () => {
      const values = new FormData(form);
      for (const key of ["title", "connectionId", "targetId", "expression", "unit"] as const) {
        if (values.has(key)) draft[key] = String(values.get(key));
      }
      for (const key of ["period", "refreshSeconds", "w", "h"] as const) {
        if (values.has(key)) draft[key] = Number(values.get(key));
      }
    };
    const setDefaultQuery = () => {
      const connection = this.getState().connections.find(c => c.id === draft.connectionId);
      const provider = this.getState().providers.find(p => p.kind === connection?.kind);
      draft.expression = provider?.queries?.[draft.kind]?.defaultExpression || (draft.kind === "metrics" ? provider?.query?.defaultExpression : "") || "";
      draft.unit = "";
    };
    if (!existing) setDefaultQuery();
    const fields = () => {
      const connection = this.getState().connections.find(c => c.id === draft.connectionId);
      const provider = this.getState().providers.find(p => p.kind === connection?.kind);
      const connections = this.compatible(draft.kind);
      const targets = (connection?.targets || []).filter(t => !t.capabilities?.length || t.capabilities.includes(draft.kind) || (draft.kind === "queue" && t.capabilities.includes("builds")));
      const query = queryKinds.has(draft.kind);
      form.innerHTML = `<header class="dialog-header"><h2 id="dashboard-widget-title">${existing ? "정보 설정" : "정보 추가"}</h2><button type="button" data-dashboard-close aria-label="닫기">×</button></header><div class="dialog-body dashboard-widget-fields">
        <div class="form-grid"><label>정보 종류<select name="kind">${[...new Set([...available, draft.kind])].map(k => `<option value="${k}" ${k === draft.kind ? "selected" : ""}>${kinds[k]}</option>`).join("")}</select></label><label>표시 이름<input name="title" maxlength="80" required value="${esc(draft.title)}"></label></div>
        <label>연결<select name="connectionId" required>${connections.some(c => c.id === draft.connectionId) ? "" : '<option value="">연결을 선택하세요</option>'}${connections.map(c => `<option value="${esc(c.id)}" ${c.id === draft.connectionId ? "selected" : ""}>${esc(c.name)}</option>`).join("")}</select></label>
        ${query ? `${draft.kind === "metrics" && connection?.rules?.length ? `<label>저장된 지표 규칙<select name="preset"><option value="">직접 입력</option>${connection.rules.map((r, i) => `<option value="${i}">${esc(r.name)}</option>`).join("")}</select></label>` : ""}<label>${esc(provider?.queries?.[draft.kind]?.language || provider?.query?.language || "조회식")}<textarea name="expression" required maxlength="16384" rows="3" spellcheck="false">${esc(draft.expression)}</textarea></label><div class="form-grid"><label>조회 범위<select name="period">${[900, 3600, 21600, 86400].map((p, i) => `<option value="${p}" ${p === draft.period ? "selected" : ""}>${["최근 15분", "최근 1시간", "최근 6시간", "최근 24시간"][i]}</option>`).join("")}</select></label><label>자동 새로고침<select name="refreshSeconds">${[0, 30, 60, 120].map(p => `<option value="${p}" ${p === draft.refreshSeconds ? "selected" : ""}>${p ? `${p}초` : "수동"}</option>`).join("")}</select></label></div>${draft.kind === "metrics" ? `<label>표시 단위<input name="unit" maxlength="24" value="${esc(draft.unit)}" placeholder="예: %, ms"></label>` : ""}<p class="muted">자동 새로고침은 이 대시보드를 열어둔 동안에만 동작합니다.</p>` : `<label>감시 대상<select name="targetId"><option value="">이 연결의 모든 감시 대상</option>${draft.targetId && !targets.some(t => t.id === draft.targetId) ? `<option selected value="${esc(draft.targetId)}">삭제된 대상 · ${esc(draft.targetId)}</option>` : ""}${targets.map(t => `<option value="${esc(t.id)}" ${t.id === draft.targetId ? "selected" : ""}>${esc(t.name)}${t.environment ? ` · ${esc(t.environment)}` : ""}</option>`).join("")}</select></label><p class="muted">연결에 설정된 수집 주기를 따릅니다.</p>`}
        <div class="form-grid"><label>가로 크기 · 12칸 기준<input name="w" type="number" min="3" max="12" required value="${draft.w}"></label><label>세로 크기<input name="h" type="number" min="2" max="8" required value="${draft.h}"></label></div><div class="bad" id="dashboard-widget-error" role="alert"></div>
      </div><footer class="dialog-footer"><button type="button" data-dashboard-close>취소</button><button type="submit" class="primary">저장</button></footer>`;
      form.querySelectorAll<HTMLButtonElement>("[data-dashboard-close]").forEach(b => b.onclick = () => dialog.close());
    };
    fields();
    form.onchange = e => {
      e.stopPropagation();
      const input = e.target as HTMLInputElement;
      capture();
      if (input.name === "kind") {
        if (input.value === "json") { dialog.close(); this.openAPI(); return; }
        draft.kind = input.value as WidgetKind; draft.connectionId = this.compatible(draft.kind)[0]?.id || "";
        draft.title = kinds[draft.kind]; draft.targetId = ""; draft.refreshSeconds = draft.kind === "metrics" ? 60 : 0;
        if (!existing) { draft.w = draft.kind === "logs" || draft.kind === "traces" ? 6 : 4; draft.h = queryKinds.has(draft.kind) ? 4 : 3; }
        setDefaultQuery(); fields();
      }
      if (input.name === "connectionId") { draft.targetId = ""; setDefaultQuery(); fields(); }
      if (input.name === "preset" && input.value !== "") {
        const rule = this.getState().connections.find(c => c.id === draft.connectionId)?.rules[Number(input.value)];
        if (rule) { draft.title = rule.name; draft.expression = rule.expression; draft.unit = rule.unit; fields(); }
      }
    };
    form.onsubmit = e => {
      e.preventDefault(); e.stopPropagation(); capture();
      try {
        if (!this.compatible(draft.kind).some(c => c.id === draft.connectionId)) throw new Error("이 정보를 지원하는 연결을 선택하세요.");
        if (queryKinds.has(draft.kind) && (!draft.expression.trim() || new TextEncoder().encode(draft.expression).length > 16384)) throw new Error("조회식을 입력하세요. 최대 16KiB까지 사용할 수 있습니다.");
        draft.title = draft.title.trim();
        draft.x = Math.min(draft.x, COLUMNS - draft.w);
        const widgets = placeWidget(this.layout.widgets, draft);
        parseLayout(JSON.stringify({ version: 2, widgets }));
        dialog.close(); this.change(widgets);
      } catch (error) { form.querySelector("#dashboard-widget-error")!.textContent = String(error); }
    };
    dialog.showModal();
  }

  private beginGesture(e: PointerEvent, w: DashboardWidget, mode: "move" | "resize") {
    if (e.button !== 0 || !this.editing || this.saving || this.compact()) return;
    e.preventDefault();
    (e.target as HTMLElement).closest<HTMLButtonElement>("button")?.focus();
    const handle = e.currentTarget as HTMLElement;
    const ghost = document.createElement("div");
    ghost.className = "dashboard-ghost";
    this.board()!.appendChild(ghost);
    this.position(ghost, w);
    this.gesture = { widget: clone(w), proposal: clone(w), mode, x: e.clientX, y: e.clientY, pointer: e.pointerId, handle, ghost };
    handle.setPointerCapture(e.pointerId);
    handle.onpointermove = event => this.drag(event);
    handle.onpointerup = () => this.finishGesture();
    handle.onpointercancel = () => this.cancelGesture();
    handle.onlostpointercapture = () => this.cancelGesture();
    document.addEventListener("keydown", this.escapeGesture);
  }

  private escapeGesture = (e: KeyboardEvent) => { if (e.key === "Escape") { e.preventDefault(); this.cancelGesture(); } };

  private drag(e: PointerEvent) {
    const g = this.gesture;
    if (!g) return;
    const dx = Math.round((e.clientX - g.x) / ((this.board()!.clientWidth + GAP) / COLUMNS));
    const dy = Math.round((e.clientY - g.y) / ROW);
    const w = clone(g.widget);
    if (g.mode === "move") { w.x = bounded(w.x + dx, 0, COLUMNS - w.w); w.y = bounded(w.y + dy, 0, 480); }
    else { w.w = bounded(w.w + dx, 3, COLUMNS - w.x); w.h = bounded(w.h + dy, 2, 8); }
    g.proposal = w;
    this.position(g.ghost, w);
    this.message(`${kinds[w.kind]} · ${w.x + 1}열 ${w.y + 1}행 · ${w.w} × ${w.h}`);
  }

  private clearGesture() {
    const g = this.gesture;
    if (!g) return;
    this.gesture = null;
    g.handle.onpointermove = g.handle.onpointerup = g.handle.onpointercancel = g.handle.onlostpointercapture = null;
    try { g.handle.releasePointerCapture(g.pointer); } catch { /* The window can lose pointer capture. */ }
    g.ghost.remove();
    document.removeEventListener("keydown", this.escapeGesture);
  }

  private cancelGesture() {
    this.clearGesture();
    this.positionCards();
  }

  private finishGesture() {
    if (!this.gesture) return;
    const { widget, proposal } = this.gesture;
    this.clearGesture();
    if ((["x", "y", "w", "h"] as const).every(k => widget[k] === proposal[k])) return;
    try { this.change(placeWidget(this.layout.widgets, proposal)); }
    catch (error) { this.report(error); this.positionCards(); }
  }

  private moveKey(e: KeyboardEvent, original: DashboardWidget, mode: "move" | "resize") {
    if (!this.editing || this.saving || this.compact() || !e.key.startsWith("Arrow")) return;
    e.preventDefault();
    const widget = clone(original);
    const dx = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
    const dy = e.key === "ArrowDown" ? 1 : e.key === "ArrowUp" ? -1 : 0;
    if (mode === "move") { widget.x = bounded(widget.x + dx, 0, COLUMNS - widget.w); widget.y = bounded(widget.y + dy, 0, 480); }
    else { widget.w = bounded(widget.w + dx, 3, COLUMNS - widget.x); widget.h = bounded(widget.h + dy, 2, 8); }
    try {
      this.change(placeWidget(this.layout.widgets, widget));
      [...this.root!.querySelectorAll<HTMLElement>(".dashboard-widget")].find(el => el.dataset.widgetId === widget.id)?.querySelector<HTMLButtonElement>(`[data-widget-action="${mode}"]`)?.focus();
      this.message(`${widget.x + 1}열 ${widget.y + 1}행 · ${widget.w} × ${widget.h} · 저장 전`);
    } catch (error) { this.report(error); }
  }
}
