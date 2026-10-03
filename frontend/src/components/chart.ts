import * as echarts from "echarts/core";
import { LineChart } from "echarts/charts";
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
} from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type { QueryResult } from "../shared/types";
echarts.use([
  LineChart,
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent,
  CanvasRenderer,
]);
let charts: echarts.ECharts[] = [];
export function dispose() {
  charts.forEach((c) => c.dispose());
  charts = [];
}
export function chart(el: HTMLElement, result: QueryResult, big = false) {
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

