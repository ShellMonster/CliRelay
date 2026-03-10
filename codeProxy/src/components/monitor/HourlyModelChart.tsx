import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Chart } from "react-chartjs-2";
import type { MonitorUsageData } from "@/modules/monitor/types";
import styles from "@/pages/MonitorPage.module.scss";

interface HourlyModelChartProps {
  data: MonitorUsageData | null;
  loading: boolean;
  isDark: boolean;
}

// 颜色调色板
const COLORS = [
  "rgba(59, 130, 246, 0.7)", // 蓝色
  "rgba(34, 197, 94, 0.7)", // 绿色
  "rgba(249, 115, 22, 0.7)", // 橙色
  "rgba(139, 92, 246, 0.7)", // 紫色
  "rgba(236, 72, 153, 0.7)", // 粉色
  "rgba(6, 182, 212, 0.7)", // 青色
];

type HourRange = 6 | 12 | 24;

export function HourlyModelChart({ data, loading, isDark }: HourlyModelChartProps) {
  const { t } = useTranslation();
  const [hourRange, setHourRange] = useState<HourRange>(12);

  // 按小时聚合数据
  const hourlyData = useMemo(() => {
    if (data?.monitor?.hourly?.length) {
      const now = new Date();
      const cutoff = new Date(now.getTime() - hourRange * 60 * 60 * 1000);
      cutoff.setMinutes(0, 0, 0);
      const allHours: string[] = [];
      for (let i = 0; i <= hourRange; i++) {
        const hourTime = new Date(cutoff.getTime() + i * 60 * 60 * 1000);
        allHours.push(hourTime.toISOString().slice(0, 13));
      }

      const hourlyStats: Record<string, Record<string, number>> = {};
      const totals: Record<string, number> = {};
      allHours.forEach((hour) => {
        hourlyStats[hour] = {};
      });

      data.monitor.hourly.forEach((item) => {
        const hourKey = item.hour.slice(0, 13);
        if (!allHours.includes(hourKey)) return;
        hourlyStats[hourKey][item.model] = (hourlyStats[hourKey][item.model] || 0) + item.requests;
        totals[item.model] = (totals[item.model] || 0) + item.requests;
      });

      const topModels = Object.entries(totals)
        .sort((a, b) => b[1] - a[1])
        .slice(0, 6)
        .map(([name]) => name);

      const modelData: Record<string, number[]> = {};
      topModels.forEach((model) => {
        modelData[model] = allHours.map((hour) => hourlyStats[hour]?.[model] || 0);
      });

      const successRates = allHours.map(() => 0);
      return { hours: allHours, models: topModels, modelData, successRates };
    }
    return { hours: [], models: [], modelData: {} as Record<string, number[]>, successRates: [] };
  }, [data, hourRange]);

  // 获取时间范围标签
  const hourRangeLabel = useMemo(() => {
    if (hourRange === 6) return t("monitor.hourly.last_6h");
    if (hourRange === 12) return t("monitor.hourly.last_12h");
    return t("monitor.hourly.last_24h");
  }, [hourRange, t]);

  // 图表数据
  const chartData = useMemo(() => {
    const labels = hourlyData.hours.map((hour) => {
      const date = new Date(hour + ":00:00Z"); // 添加 Z 表示 UTC 时间，确保正确转换为本地时间显示
      return `${date.getHours()}:00`;
    });

    // 成功率折线放在最前面
    const datasets: any[] = [
      {
        type: "line" as const,
        label: t("monitor.hourly.success_rate"),
        data: hourlyData.successRates,
        borderColor: "#4ef0c3",
        backgroundColor: "#4ef0c3",
        borderWidth: 2.5,
        tension: 0.4,
        yAxisID: "y1",
        stack: "",
        pointRadius: 3,
        pointBackgroundColor: "#4ef0c3",
        pointBorderColor: "#4ef0c3",
      },
    ];

    // 添加模型柱状图
    hourlyData.models.forEach((model, index) => {
      datasets.push({
        type: "bar" as const,
        label: model,
        data: hourlyData.modelData[model],
        backgroundColor: COLORS[index % COLORS.length],
        borderColor: COLORS[index % COLORS.length],
        borderWidth: 1,
        borderRadius: 4,
        stack: "models",
        yAxisID: "y",
      });
    });

    return { labels, datasets };
  }, [hourlyData, t]);

  // 图表配置
  const chartOptions = useMemo(
    () => ({
      responsive: true,
      maintainAspectRatio: false,
      interaction: {
        mode: "index" as const,
        intersect: false,
      },
      plugins: {
        legend: {
          display: true,
          position: "bottom" as const,
          labels: {
            color: isDark ? "#9ca3af" : "#6b7280",
            usePointStyle: true,
            padding: 12,
            font: {
              size: 11,
            },
            generateLabels: (chart: any) => {
              return chart.data.datasets.map((dataset: any, i: number) => {
                const isLine = dataset.type === "line";
                return {
                  text: dataset.label,
                  fillStyle: dataset.backgroundColor,
                  strokeStyle: dataset.borderColor,
                  lineWidth: 0,
                  hidden: !chart.isDatasetVisible(i),
                  datasetIndex: i,
                  pointStyle: isLine ? "circle" : "rect",
                };
              });
            },
          },
        },
        tooltip: {
          backgroundColor: isDark ? "#374151" : "#ffffff",
          titleColor: isDark ? "#f3f4f6" : "#111827",
          bodyColor: isDark ? "#d1d5db" : "#4b5563",
          borderColor: isDark ? "#4b5563" : "#e5e7eb",
          borderWidth: 1,
          padding: 12,
        },
      },
      scales: {
        x: {
          stacked: true,
          grid: {
            color: isDark ? "rgba(255, 255, 255, 0.06)" : "rgba(0, 0, 0, 0.06)",
          },
          ticks: {
            color: isDark ? "#9ca3af" : "#6b7280",
            font: {
              size: 11,
            },
          },
        },
        y: {
          stacked: true,
          position: "left" as const,
          grid: {
            color: isDark ? "rgba(255, 255, 255, 0.06)" : "rgba(0, 0, 0, 0.06)",
          },
          ticks: {
            color: isDark ? "#9ca3af" : "#6b7280",
            font: {
              size: 11,
            },
          },
          title: {
            display: true,
            text: t("monitor.hourly.requests"),
            color: isDark ? "#9ca3af" : "#6b7280",
            font: {
              size: 11,
            },
          },
        },
        y1: {
          position: "right" as const,
          min: 0,
          max: 100,
          grid: {
            drawOnChartArea: false,
          },
          ticks: {
            color: isDark ? "#9ca3af" : "#6b7280",
            font: {
              size: 11,
            },
            callback: (value: string | number) => `${value}%`,
          },
          title: {
            display: true,
            text: t("monitor.hourly.success_rate"),
            color: isDark ? "#9ca3af" : "#6b7280",
            font: {
              size: 11,
            },
          },
        },
      },
    }),
    [isDark, t],
  );

  return (
    <div className={styles.chartCard}>
      <div className={styles.chartHeader}>
        <div>
          <h3 className={styles.chartTitle}>{t("monitor.hourly_model.title")}</h3>
          <p className={styles.chartSubtitle}>{hourRangeLabel}</p>
        </div>
        <div className={styles.chartControls}>
          <button
            className={`${styles.chartControlBtn} ${hourRange === 6 ? styles.active : ""}`}
            onClick={() => setHourRange(6)}
          >
            {t("monitor.hourly.last_6h")}
          </button>
          <button
            className={`${styles.chartControlBtn} ${hourRange === 12 ? styles.active : ""}`}
            onClick={() => setHourRange(12)}
          >
            {t("monitor.hourly.last_12h")}
          </button>
          <button
            className={`${styles.chartControlBtn} ${hourRange === 24 ? styles.active : ""}`}
            onClick={() => setHourRange(24)}
          >
            {t("monitor.hourly.last_24h")}
          </button>
        </div>
      </div>

      <div className={styles.chartContent}>
        {loading || hourlyData.hours.length === 0 ? (
          <div className={styles.chartEmpty}>
            {loading ? t("common.loading") : t("monitor.no_data")}
          </div>
        ) : (
          <Chart type="bar" data={chartData} options={chartOptions} />
        )}
      </div>
    </div>
  );
}
