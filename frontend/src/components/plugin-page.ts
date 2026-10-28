import "./extensions.css";
import { api } from "../bridge";
import type { State } from "../shared/types";
import type { CardContribution, QueryContribution, ExtensionBinding, SourceState } from "../bridge/extensions";
import { esc } from "../shared/format";
import { plugins, loadPlugins, schemaForm, readSchema, renderData } from "./extensions";
import type { DashboardWidget } from "./dashboard-layout";

export async function mountPlugins(root: HTMLElement, state: State, alive: () => boolean, add: (binding: ExtensionBinding, connection: string, title: string, existing?: string) => Promise<void>, refresh: () => Promise<void>, pluginId?: string, dashboardId?: string, editing?: DashboardWidget) {
  const plugin = plugins.find(p => p.manifest.id === pluginId);
  if (!plugin) {
    root.innerHTML = `<section class="panel extension-panel"><h2>플러그인</h2><p>플러그인의 대시보드와 조회를 열어 내 대시보드에 정보를 추가하세요.</p>${plugins.map(p => `<article class="panel extension-panel"><h3>${esc(p.manifest.id)} <small>${esc(p.manifest.version)}</small></h3><button data-plugin-toggle="${esc(p.manifest.id)}">${p.enabled ? "비활성화" : "활성화"}</button>${(p.manifest.dashboards || []).map(d => `<button data-page="plugin:${esc(p.manifest.id)}:${esc(d.id)}">${esc(d.title)}</button>`).join("")}<button data-page="plugin:${esc(p.manifest.id)}">카드 · Query</button></article>`).join("") || '<p>등록된 플러그인이 없습니다.</p>'}<p role="alert" id="plugin-error"></p></section>`;
    root.querySelectorAll<HTMLButtonElement>("[data-plugin-toggle]").forEach(button => button.onclick = async () => {
      button.disabled = true;
      try { const p = plugins.find(p => p.manifest.id === button.dataset.pluginToggle)!; await api.SetPluginEnabled(p.manifest.id, !p.enabled); await loadPlugins(); if (alive()) await refresh(); }
      catch (e) { if (alive()) { root.querySelector("#plugin-error")!.textContent = String(e); button.disabled = false; } }
    });
    return;
  }
  const dashboard = plugin.manifest.dashboards?.find(d => d.id === dashboardId);
  const cards = (plugin.manifest.cards || []).filter(c => !dashboard || dashboard.cards.includes(c.id));
  const entries: {kind: "card" | "query"; value: CardContribution | QueryContribution}[] = [...cards.map(value => ({kind: "card" as const, value})), ...(plugin.manifest.queries || []).map(value => ({kind: "query" as const, value}))];
  root.innerHTML = `<section><header class="page-head"><h2>${esc(dashboard?.title || plugin.manifest.id)}</h2><button data-page="plugins">플러그인 관리</button></header>${plugin.enabled ? "" : '<p class="notice warning">플러그인이 비활성화되어 있습니다. 저장된 카드는 유지됩니다.</p>'}<div id="plugin-contributions" class="extension-grid"></div></section>`;
  const container = root.querySelector<HTMLElement>("#plugin-contributions")!;
  for (const {kind, value} of entries) {
    const panel = document.createElement("section"); panel.className = "panel extension-panel"; container.append(panel);
    const schema = "inputSchema" in value ? value.inputSchema : (value as CardContribution).configSchema;
    const matching = editing && editing.extension && editing.extension.pluginId === pluginId && editing.extension.id === value.id && editing.extension.kind === kind ? editing : undefined;
    const connections = state.connections.filter(c => state.providers.find(p => p.kind === c.kind)?.capabilities.includes(value.capability));
    let sources: SourceState[] = []; let generation = 0;
    panel.innerHTML = `<h3>${esc(value.title)} <small>${kind === "query" ? "Query" : "카드"}</small></h3><form><div class="form-grid"><label>연결<select data-connection required>${connections.map(c => `<option value="${esc(c.id)}" ${matching?.connectionId === c.id ? "selected" : ""}>${esc(c.name)}</option>`).join("")}</select></label><label>Data source<select data-source required></select></label><div data-inputs class="form-grid">${schemaForm(schema, matching?.extension?.input)}</div></div><div class="actions"><button type="submit" ${!plugin.enabled ? "disabled" : ""}>조회</button><button type="button" data-add-extension ${!plugin.enabled ? "disabled" : ""}>${matching ? "카드 수정" : "내 대시보드에 추가"}</button></div></form><div data-result role="status"></div>`;
    const form = panel.querySelector("form")!;
    const select = panel.querySelector<HTMLSelectElement>("[data-connection]")!;
    const source = panel.querySelector<HTMLSelectElement>("[data-source]")!;
    const output = panel.querySelector<HTMLElement>("[data-result]")!;
    const request = (): ExtensionBinding => ({pluginId: plugin.manifest.id, kind, id: value.id, sourceId: source.value, input: readSchema(form, schema)});
    const query = async () => {
      const token = ++generation; const connection = select.value, binding = request();
      if (!plugin.enabled || !form.reportValidity()) return;
      output.textContent = "조회 중…";
      try { const result = await api.ExecuteContribution(connection, binding.sourceId, binding); if (alive() && token === generation) output.innerHTML = `<p class="db-meta">${esc(select.selectedOptions[0]?.textContent || connection)} · ${esc(source.selectedOptions[0]?.textContent || binding.sourceId)} · ${esc(new Date().toLocaleTimeString())}</p>${renderData(result)}`; }
      catch (e) { if (alive() && token === generation) output.textContent = String(e); }
    };
    const connect = async () => {
      const token = ++generation; source.innerHTML = ""; output.textContent = "";
      if (!select.value || !plugin.enabled) return;
      try {
        sources = (await api.Sources(select.value)).filter(s => s.capabilities.includes(value.capability));
        if (!alive() || token !== generation) return;
        source.innerHTML = sources.map(s => `<option value="${esc(s.id)}" ${matching?.extension?.sourceId === s.id ? "selected" : ""}>${esc(s.name)}</option>`).join("");
        const target = form.querySelector<HTMLInputElement>('[data-field="targetId"]');
        if (target && !target.value) target.value = state.connections.find(c => c.id === select.value)?.targets[0]?.id || "";
        if (kind === "card" && form.checkValidity()) void query();
      } catch (e) { if (alive() && token === generation) output.textContent = String(e); }
    };
    select.onchange = () => void connect(); source.onchange = () => { generation++; output.textContent = ""; };
    form.oninput = () => { generation++; };
    form.onsubmit = event => { event.preventDefault(); void query(); };
    panel.querySelector<HTMLButtonElement>("[data-add-extension]")!.onclick = async () => {
      if (!form.reportValidity() || !sources.some(s => s.id === source.value)) return;
      try { await add(request(), select.value, value.title, matching?.id); }
      catch (e) { if (alive()) output.textContent = String(e); }
    };
    void connect();
  }
}
