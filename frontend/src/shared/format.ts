export const esc = (s: unknown) =>
  String(s ?? "").replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ]!,
  );
export const date = (v: number | string) =>
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
export const elapsed = (ms: number) =>
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
export const badge = (status: string) =>
  `<span class="badge ${cls(status)}">${esc(status || "Unknown")}</span>`;
