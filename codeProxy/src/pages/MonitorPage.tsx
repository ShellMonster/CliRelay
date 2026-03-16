// Deprecated: 默认前端入口已切换到 `src/app/AppRouter.tsx` + `src/modules/monitor/MonitorPage.tsx`。
// 这里保留仅用于历史兼容；新增功能或修复请优先修改 modules 目录下的新监控页。
import { useState, useEffect, useMemo, useCallback } from "react";
import { useTranslation } from "react-i18next";
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  BarController,
  LineController,
  ArcElement,
  Title,
  Tooltip,
  Legend,
  Filler,
} from "chart.js";
import { Button } from "@/components/ui/Button";
import { LoadingSpinner } from "@/components/ui/LoadingSpinner";
import { useHeaderRefresh } from "@/hooks/useHeaderRefresh";
import { useThemeStore } from "@/stores";
import { providersApi, usageApi } from "@/lib/http/apis";
import { KpiCards } from "@/components/monitor/KpiCards";
import { ModelDistributionChart } from "@/components/monitor/ModelDistributionChart";
import { DailyTrendChart } from "@/components/monitor/DailyTrendChart";
import { HourlyModelChart } from "@/components/monitor/HourlyModelChart";
import { HourlyTokenChart } from "@/components/monitor/HourlyTokenChart";
import { ChannelStats } from "@/components/monitor/ChannelStats";
import { FailureAnalysis } from "@/components/monitor/FailureAnalysis";
import { RequestLogs } from "@/components/monitor/RequestLogs";
import type { MonitorUsageData } from "@/modules/monitor/types";
import styles from "./MonitorPage.module.scss";

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  BarController,
  LineController,
  ArcElement,
  Title,
  Tooltip,
  Legend,
  Filler,
);

type TimeRange = 1 | 7 | 14 | 30;

const createEmptyUsageData = (): MonitorUsageData => ({
  summary: {
    TotalRequests: 0,
    SuccessRequests: 0,
    FailedRequests: 0,
    SuccessRate: 0,
    InputTokens: 0,
    OutputTokens: 0,
    ReasoningTokens: 0,
    CachedTokens: 0,
    TotalTokens: 0,
    ProcessedTokens: 0,
  },
  monitor: {
    modelDistribution: [],
    dailyTrend: [],
    hourly: [],
    channelStats: { days: 7, channels: [], models: [] },
    failureStats: { days: 7, channels: [], models: [] },
  },
});

export function MonitorPage() {
  const { t } = useTranslation();
  const resolvedTheme = useThemeStore((state) => state.resolvedTheme);
  const isDark = resolvedTheme === "dark";

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [usageData, setUsageData] = useState<MonitorUsageData>(createEmptyUsageData);
  const [timeRange, setTimeRange] = useState<TimeRange>(7);
  const [apiFilter, setApiFilter] = useState("");
  const [providerMap, setProviderMap] = useState<Record<string, string>>({});
  const [providerModels, setProviderModels] = useState<Record<string, Set<string>>>({});
  const [providerTypeMap, setProviderTypeMap] = useState<Record<string, string>>({});

  const loadProviderMap = useCallback(async () => {
    try {
      const map: Record<string, string> = {};
      const modelsMap: Record<string, Set<string>> = {};
      const typeMap: Record<string, string> = {};

      const [openaiProviders, geminiKeys, claudeConfigs, codexConfigs, vertexConfigs] =
        await Promise.all([
          providersApi.getOpenAIProviders().catch(() => []),
          providersApi.getGeminiKeys().catch(() => []),
          providersApi.getClaudeConfigs().catch(() => []),
          providersApi.getCodexConfigs().catch(() => []),
          providersApi.getVertexConfigs().catch(() => []),
        ]);

      openaiProviders.forEach((provider) => {
        const providerName = provider.headers?.["X-Provider"] || provider.name || "unknown";
        const modelSet = new Set<string>();
        (provider.models || []).forEach((m) => {
          if (m.alias) modelSet.add(m.alias);
          if (m.name) modelSet.add(m.name);
        });
        (provider.apiKeyEntries || []).forEach((entry) => {
          if (!entry.apiKey) return;
          map[entry.apiKey] = providerName;
          modelsMap[entry.apiKey] = modelSet;
          typeMap[entry.apiKey] = "OpenAI";
        });
        if (provider.name) {
          map[provider.name] = providerName;
          modelsMap[provider.name] = modelSet;
          typeMap[provider.name] = "OpenAI";
        }
      });

      geminiKeys.forEach((config) => {
        if (!config.apiKey) return;
        map[config.apiKey] = config.prefix?.trim() || "Gemini";
        typeMap[config.apiKey] = "Gemini";
      });

      claudeConfigs.forEach((config) => {
        if (!config.apiKey) return;
        map[config.apiKey] = config.prefix?.trim() || "Claude";
        typeMap[config.apiKey] = "Claude";
        if (config.models?.length) {
          const modelSet = new Set<string>();
          config.models.forEach((m) => {
            if (m.alias) modelSet.add(m.alias);
            if (m.name) modelSet.add(m.name);
          });
          modelsMap[config.apiKey] = modelSet;
        }
      });

      codexConfigs.forEach((config) => {
        if (!config.apiKey) return;
        map[config.apiKey] = config.prefix?.trim() || "Codex";
        typeMap[config.apiKey] = "Codex";
        if (config.models?.length) {
          const modelSet = new Set<string>();
          config.models.forEach((m) => {
            if (m.alias) modelSet.add(m.alias);
            if (m.name) modelSet.add(m.name);
          });
          modelsMap[config.apiKey] = modelSet;
        }
      });

      vertexConfigs.forEach((config) => {
        if (!config.apiKey) return;
        map[config.apiKey] = config.prefix?.trim() || "Vertex";
        typeMap[config.apiKey] = "Vertex";
        if (config.models?.length) {
          const modelSet = new Set<string>();
          config.models.forEach((m) => {
            if (m.alias) modelSet.add(m.alias);
            if (m.name) modelSet.add(m.name);
          });
          modelsMap[config.apiKey] = modelSet;
        }
      });

      setProviderMap(map);
      setProviderModels(modelsMap);
      setProviderTypeMap(typeMap);
    } catch (err) {
      console.warn("Monitor: Failed to load provider map:", err);
    }
  }, []);

  const loadData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [summaryRes, distributionRes, trendRes, hourlyRes, channelStatsRes, failureStatsRes] = await Promise.all([
        usageApi.getMonitorSummary(timeRange, apiFilter),
        usageApi.getMonitorModelDistribution(timeRange, 10, apiFilter),
        usageApi.getMonitorDailyTrend(timeRange, apiFilter),
        usageApi.getMonitorHourly(24, apiFilter),
        usageApi.getMonitorChannelStats(timeRange, 10, apiFilter),
        usageApi.getMonitorFailureStats(timeRange, 10, apiFilter),
        loadProviderMap(),
      ]);

      const nextData = createEmptyUsageData();
      nextData.summary = summaryRes.summary;
      nextData.monitor = {
        modelDistribution: distributionRes.items,
        dailyTrend: trendRes.items,
        hourly: hourlyRes.items,
        channelStats: channelStatsRes,
        failureStats: failureStatsRes,
      };

      setUsageData(nextData);
    } catch (err) {
      const message = err instanceof Error ? err.message : t("common.unknown_error");
      console.error("Monitor: Error loading data:", err);
      setError(message);
    } finally {
      setLoading(false);
    }
  }, [apiFilter, loadProviderMap, t, timeRange]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  useHeaderRefresh(loadData);

  const filteredData = useMemo(() => usageData, [usageData]);

  const handleTimeRangeChange = (range: TimeRange) => {
    setTimeRange(range);
  };

  const handleApiFilterApply = () => {
    void loadData();
  };

  return (
    <div className={styles.container}>
      {loading && !usageData.summary && (
        <div className={styles.loadingOverlay} aria-busy="true">
          <div className={styles.loadingOverlayContent}>
            <LoadingSpinner size={28} className={styles.loadingOverlaySpinner} />
            <span className={styles.loadingOverlayText}>{t("common.loading")}</span>
          </div>
        </div>
      )}

      <div className={styles.header}>
        <h1 className={styles.pageTitle}>{t("monitor.title")}</h1>
        <div className={styles.headerActions}>
          <Button variant="secondary" size="sm" onClick={loadData} disabled={loading}>
            {loading ? t("common.loading") : t("common.refresh")}
          </Button>
        </div>
      </div>

      {error && <div className={styles.errorBox}>{error}</div>}

      <div className={styles.filters}>
        <div className={styles.filterGroup}>
          <span className={styles.filterLabel}>{t("monitor.time_range")}</span>
          <div className={styles.timeButtons}>
            {([1, 7, 14, 30] as TimeRange[]).map((range) => (
              <button
                key={range}
                className={`${styles.timeButton} ${timeRange === range ? styles.active : ""}`}
                onClick={() => handleTimeRangeChange(range)}
              >
                {range === 1 ? t("monitor.today") : t("monitor.last_n_days", { n: range })}
              </button>
            ))}
          </div>
        </div>
        <div className={styles.filterGroup}>
          <span className={styles.filterLabel}>{t("monitor.api_filter")}</span>
          <input
            type="text"
            className={styles.filterInput}
            placeholder={t("monitor.api_filter_placeholder")}
            value={apiFilter}
            onChange={(e) => setApiFilter(e.target.value)}
          />
          <Button variant="secondary" size="sm" onClick={handleApiFilterApply}>
            {t("monitor.apply")}
          </Button>
        </div>
      </div>

      <KpiCards data={filteredData} loading={loading} timeRange={timeRange} />

      <div className={styles.chartsGrid}>
        <ModelDistributionChart
          data={filteredData}
          loading={loading}
          isDark={isDark}
          timeRange={timeRange}
        />
        <DailyTrendChart
          data={filteredData}
          loading={loading}
          isDark={isDark}
          timeRange={timeRange}
        />
      </div>

      <HourlyModelChart data={filteredData} loading={loading} isDark={isDark} />
      <HourlyTokenChart data={filteredData} loading={loading} isDark={isDark} />

      <div className={styles.statsGrid}>
        <ChannelStats
          data={filteredData}
          loading={loading}
          providerMap={providerMap}
          providerModels={providerModels}
        />
        <FailureAnalysis
          data={filteredData}
          loading={loading}
          providerMap={providerMap}
          providerModels={providerModels}
        />
      </div>

      <RequestLogs
        data={filteredData}
        loading={loading}
        providerMap={providerMap}
        providerTypeMap={providerTypeMap}
        apiFilter={apiFilter}
      />
    </div>
  );
}
