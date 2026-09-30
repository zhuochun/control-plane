import { useEffect, useRef } from "react";
import {
  Chart,
  CategoryScale,
  LinearScale,
  LineController,
  LineElement,
  PointElement,
  Filler,
  Tooltip,
} from "chart.js";
import { type Item, parseMetric } from "./items";

Chart.register(
  CategoryScale,
  LinearScale,
  LineController,
  LineElement,
  PointElement,
  Filler,
  Tooltip,
);

export function MetricChart({ item }: { item: Item }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const metric = parseMetric(item);
  useEffect(() => {
    if (!canvas.current || !metric) return;
    const points = metric.series;
    const formatValue = (value: number) => {
      const currency = metric.current.display.match(/^[-+]?([$€£¥])/);
      const unit = metric.current.display.match(/[a-zA-Z%]+$/);
      return (
        (currency?.[1] ?? "") +
        new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(
          value,
        ) +
        (unit ? " " + unit[0] : "")
      );
    };
    const chart = new Chart(canvas.current, {
      type: "line",
      data: {
        labels: points.map(
          (point, index) =>
            point.label ??
            (index === 0
              ? "Previous"
              : index === points.length - 1
                ? "Latest"
                : "Observation " + (index + 1)),
        ),
        datasets: [
          {
            label: metric.label,
            data: points.map((point) => point.value),
            borderColor: "#246b59",
            backgroundColor: "rgba(36,107,89,.08)",
            borderWidth: 2,
            pointRadius: 4,
            pointBackgroundColor: "#246b59",
            pointHoverRadius: 6,
            fill: true,
          },
          ...(metric.target
            ? [
                {
                  label: "Target " + metric.target.display,
                  data: points.map(() => metric.target!.value),
                  borderColor: "#818981",
                  borderDash: [6, 5],
                  borderWidth: 1,
                  pointRadius: 0,
                  fill: false,
                },
              ]
            : []),
        ],
      },
      plugins: [{
        id: "observationLabels",
        afterDatasetsDraw: (chart) => {
          const context = chart.ctx;
          context.save();
          context.font = '13px "Segoe UI", sans-serif';
          context.fillStyle = "#252e2a";
          context.textAlign = "center";
          chart.getDatasetMeta(0).data.forEach((point, index) => {
            context.fillText(points[index].display, point.x, point.y - 12);
          });
          context.restore();
        },
      }],
      options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        interaction: { mode: "index", intersect: false },
        layout: { padding: { top: 24, right: 24, left: 12 } },
        plugins: {
          tooltip: {
            callbacks: {
              label: (context) =>
                context.dataset.label +
                ": " +
                formatValue(context.parsed.y ?? 0),
            },
          },
        },
        scales: {
          x: {
            grid: { color: "#edf0eb" },
            ticks: {
              color: "#697369",
              font: { size: 13 },
              maxRotation: 0,
              autoSkip: true,
            },
          },
          y: {
            grace: "15%",
            grid: { color: "#edf0eb" },
            border: { display: false },
            ticks: {
              color: "#697369",
              font: { size: 13 },
              maxTicksLimit: 5,
              callback: (value) => formatValue(Number(value)),
            },
          },
        },
      },
    });
    return () => chart.destroy();
  }, [item.summary, item.report.body_md]);
  if (!metric) return null;
  return (
    <section className="reader-chart" aria-label="Metric trend">
      <div className="reader-chart-heading">
        <span>{metric.label}</span>
        {metric.target && <span>Target {metric.target.display}</span>}
      </div>
      <div className="reader-chart-canvas">
        <canvas
          ref={canvas}
          role="img"
          aria-label={
            metric.label +
            ": " +
            metric.series
              .map(
                (point) =>
                  (point.label ? point.label + " " : "") + point.display,
              )
              .join(", ") +
            (metric.target ? "; target " + metric.target.display : "")
          }
        />
      </div>
    </section>
  );
}
