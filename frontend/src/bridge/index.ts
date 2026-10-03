import type { Bridge } from "./types";
import { previewBridge } from "./preview";

// Only this boundary selects native Wails access versus the browser fixture.
export const api: Bridge =
  window.go?.main.App ??
  previewBridge(new URLSearchParams(location.search).has("empty"));
export const browserPreview = !window.go;
