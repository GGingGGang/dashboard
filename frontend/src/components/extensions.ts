import { api } from "../bridge";
import type { PluginState, ExtensionBinding, DataResult, FormSchema } from "../bridge/extensions";
import { esc } from "../shared/format";

export let plugins: PluginState[] = [];
export async function loadPlugins() {
  // Older bridges can still display their saved built-in cards.
  plugins = typeof api.Plugins === "function" ? await api.Plugins() : [];
}
export function contribution(binding: ExtensionBinding) {
  const plugin = plugins.find(p => p.manifest.id === binding.pluginId);
  if (!plugin?.enabled) return undefined;
  return (binding.kind === "card" ? plugin.manifest.cards : plugin.manifest.queries)?.find(c => c.id === binding.id);
}
export function extensionKey(binding: ExtensionBinding) {
  return JSON.stringify([binding, plugins.find(p => p.manifest.id === binding.pluginId)]);
}
export function schemaForm(schema?: FormSchema | null, values: Record<string, unknown> = {}) {
  return Object.entries(schema?.properties || {}).map(([name, field]) => {
    const value = values[name] ?? field.default ?? (field.type === "boolean" ? false : "");
    const required = field.type !== "boolean" && schema?.required?.includes(name) ? "required" : "";
    const attrs = `data-field="${esc(name)}" ${required}`;
    const input = field.enum ? `<select ${attrs}>${field.enum.map((item, i) => `<option value="${i}" ${item === value ? "selected" : ""}>${esc(item)}</option>`).join("")}</select>`
      : field.type === "boolean" ? `<input ${attrs} type="checkbox" ${value ? "checked" : ""}>`
      : `<input ${attrs} type="${field.type === "string" ? "text" : "number"}" value="${esc(value)}" ${field.type === "string" ? `maxlength="${field.maxLength ?? 16384}"` : `step="${field.type === "integer" ? 1 : "any"}" ${field.minimum === undefined ? "" : `min="${field.minimum}"`} ${field.maximum === undefined ? "" : `max="${field.maximum}"`}`}>`;
    return `<label>${esc(field.title || name)}${input}<small>${esc(field.description || "")}</small></label>`;
  }).join("");
}
export function readSchema(root: HTMLElement, schema?: FormSchema | null): Record<string, unknown> {
  const values: Record<string, unknown> = {};
  root.querySelectorAll<HTMLInputElement | HTMLSelectElement>("[data-field]").forEach(el => {
    const name = el.dataset.field!; const f = schema?.properties[name]; if (!f) return;
    if (f.type !== "boolean" && el.value === "" && !schema?.required?.includes(name)) return;
    values[name] = f.enum ? f.enum[Number(el.value)] : f.type === "boolean" ? (el as HTMLInputElement).checked : f.type === "string" ? el.value : Number(el.value);
  });
  return values;
}
export function renderData(result: DataResult): string {
  const warnings = (result.warnings || []).map(w => `<p class="notice warning">${esc(w)}</p>`).join("");
  if (result.type === "scalar") return `${warnings}<div class="db-current">${esc(result.value ?? "—")}</div>`;
  if (result.type === "table") return `${warnings}<div class="table-wrap"><table><thead><tr>${(result.columns || []).map(c => `<th>${esc(c.name)}</th>`).join("")}</tr></thead><tbody>${(result.rows || []).map(row => `<tr>${row.map(v => `<td>${esc(v ?? "—")}</td>`).join("")}</tr>`).join("")}</tbody></table>${!result.rows?.length ? '<p class="empty">결과가 없습니다.</p>' : ""}</div>`;
  return warnings + (result.series || []).slice(0, 100).map(series => {
    const points = series.points.filter(p => p.value !== null && Number.isFinite(p.value) && Number.isFinite(p.time));
    const low = Math.min(...points.map(p => p.value!)), high = Math.max(...points.map(p => p.value!));
    const start = Math.min(...points.map(p => p.time)), end = Math.max(...points.map(p => p.time));
    const segments: string[][] = [[]];
    for (const point of series.points) {
      if (point.value === null || !Number.isFinite(point.value) || !Number.isFinite(point.time)) { if (segments.at(-1)!.length) segments.push([]); continue; }
      segments.at(-1)!.push(`${10 + 580 * (point.time - start) / (end - start || 1)},${110 - 100 * (point.value - low) / (high - low || 1)}`);
    }
    const graph = segments.filter(segment => segment.length).map(segment => segment.length === 1 ? `<circle cx="${segment[0].split(",")[0]}" cy="${segment[0].split(",")[1]}" r="3" fill="currentColor"/>` : `<polyline points="${segment.join(" ")}" fill="none" stroke="currentColor" stroke-width="2"/>`).join("");
    return `<figure><figcaption>${esc(Object.entries(series.labels || {}).map(([k,v]) => `${k}=${v}`).join(", ") || "value")} · ${esc(points.at(-1)?.value ?? "—")}</figcaption><svg viewBox="0 0 600 120" role="img" aria-label="시계열 그래프">${graph}</svg><details><summary>데이터 ${points.length}개</summary><pre>${esc(points.map(p => `${new Date(p.time * 1000).toISOString()}  ${p.value}`).join("\n"))}</pre></details></figure>`;
  }).join("");
}
