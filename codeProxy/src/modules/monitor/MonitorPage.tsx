import { useCallback, useEffect, useMemo, useRef, useState, useTransition } from "react";
import { Activity, ChartSpline, Coins, Filter, RefreshCw, ShieldCheck, Sigma } from "lucide-react";
import { usageApi, type MonitorFiltersResponse } from "@/lib/http/apis/usage";
import { formatNumber, formatRate } from "@/modules/monitor/monitor-utils";
import { AnimatedNumber } from "@/modules/ui/AnimatedNumber";
import { MultiSelect } from "@/modules/ui/MultiSelect";
import { Reveal } from "@/modules/ui/Reveal";
import { SearchableSelect } from "@/modules/ui/SearchableSelect";
import { EChart } from "@/modules/ui/charts/EChart";
import { ChartLegend } from "@/modules/ui/charts/ChartLegend";
import { useTheme } from "@/modules/ui/ThemeProvider";
import type { HourWindow, TimeRange } from "@/modules/monitor/monitor-constants";
import { CHART_COLOR_CLASSES, HOURLY_MODEL_COLORS } from "@/modules/monitor/monitor-constants";
import { formatCompact, formatMonthDay } from "@/modules/monitor/monitor-format";
import {
  HourWindowSelector,
  KpiCard,
  MonitorCard as Card,
  TimeRangeSelector,
} from "@/modules/monitor/MonitorPagePieces";
import {
  createDailyTrendOption,
  createHourlyModelOption,
  createHourlyTokenOption,
  createModelDistributionOption,
} from "@/modules/monitor/monitor-chart-options";
import { Tabs, TabsList, TabsTrigger } from "@/modules/ui/Tabs";
import { useToast } from "@/modules/ui/ToastProvider";

const EMPTY_FILTER_OPTIONS: MonitorFiltersResponse["filters"] = {
  api_keys: [],
  api_key_names: {},
  models: [],
  channels: [],
  channel_options: [],
};

type MonitorSummaryResult = Awaited<ReturnType<typeof usageApi.getMonitorSummary>>;
type MonitorDistributionResult = Awaited<ReturnType<typeof usageApi.getMonitorModelDistribution>>;
type MonitorDailyResult = Awaited<ReturnType<typeof usageApi.getMonitorDailyTrend>>;
type MonitorHourlyResult = Awaited<ReturnType<typeof usageApi.getMonitorHourly>>;

const monitorAutoRequestCache = {
  filters: new Map<string, Promise<MonitorFiltersResponse>>(),
  summary: new Map<string, Promise<MonitorSummaryResult>>(),
  charts: new Map<
    string,
    Promise<{
      distributionRes: MonitorDistributionResult;
      dailyRes: MonitorDailyResult;
      hourlyRes: MonitorHourlyResult;
    }>
  >(),
};

function getCachedMonitorRequest<T>(
  cache: Map<string, Promise<T>>,
  key: string,
  factory: () => Promise<T>,
): Promise<T> {
  const cached = cache.get(key);
  if (cached) {
    return cached;
  }
  const request = factory().finally(() => {
    cache.delete(key);
  });
  cache.set(key, request);
  return request;
}

function normalizeFilterValue(value: string) {
  return value.trim();
}

function normalizeFilterList(values: string[]) {
  return values.map((item) => item.trim()).filter(Boolean);
}

export function MonitorPage() {
  const { notify } = useToast();
  const {
    state: { mode },
  } = useTheme();
  const isDark = mode === "dark";
  const [dailyLegendSelected, setDailyLegendSelected] = useState<Record<string, boolean>>({
    "输入 Token": true,
    "输出 Token": true,
    请求数: true,
  });

  const [hourlyModelSelected, setHourlyModelSelected] = useState<Record<string, boolean>>({
    总请求: true,
  });

  const [hourlyTokenSelected, setHourlyTokenSelected] = useState<Record<string, boolean>>({
    输入: true,
    输出: true,
    推理: true,
    缓存: true,
    "处理量 Token": true,
  });

  const [timeRange, setTimeRange] = useState<TimeRange>(7);
  const [filterOptions, setFilterOptions] =
    useState<MonitorFiltersResponse["filters"]>(EMPTY_FILTER_OPTIONS);
  const [pendingApiFilter, setPendingApiFilter] = useState("");
  const [pendingModelFilter, setPendingModelFilter] = useState("");
  const [pendingChannelFilter, setPendingChannelFilter] = useState<string[]>([]);
  const [apiFilter, setApiFilter] = useState("");
  const [modelFilter, setModelFilter] = useState("");
  const [channelFilter, setChannelFilter] = useState<string[]>([]);
  const [modelHourWindow, setModelHourWindow] = useState<HourWindow>(24);
  const [tokenHourWindow, setTokenHourWindow] = useState<HourWindow>(24);
  const [modelMetric, setModelMetric] = useState<"requests" | "processed">("requests");
  const [error, setError] = useState<string | null>(null);
  const [isSummaryRefreshing, setIsSummaryRefreshing] = useState(true);
  const [isChartsRefreshing, setIsChartsRefreshing] = useState(false);
  const [isPending, startTransition] = useTransition();
  const summaryRequestIdRef = useRef(0);
  const chartRequestIdRef = useRef(0);
  const filterRequestIdRef = useRef(0);
  const lastAutoSummaryKeyRef = useRef<string | null>(null);
  const lastAutoChartKeyRef = useRef<string | null>(null);
  const lastAutoFilterKeyRef = useRef<string | null>(null);
  const [deferredChartsRequest, setDeferredChartsRequest] = useState<{
    key: string;
    nonce: number;
    force: boolean;
  } | null>(null);
  const [summary, setSummary] = useState({
    requestCount: 0,
    successCount: 0,
    failedCount: 0,
    successRate: 0,
    processedTokens: 0,
    totalTokens: 0,
    cachedTokens: 0,
    inputTokens: 0,
    outputTokens: 0,
  });
  const [modelDistributionItems, setModelDistributionItems] = useState<MonitorDistributionResult["items"]>(
    [],
  );
  const [dailySeries, setDailySeries] = useState<
    Array<{
      label: string;
      requests: number;
      inputTokens: number;
      outputTokens: number;
      totalTokens: number;
    }>
  >([]);
  const [hourlySeries, setHourlySeries] = useState<{
    modelKeys: string[];
    modelPoints: Array<{
      label: string;
      stacks: Array<{ key: string; value: number }>;
      successRate: number;
    }>;
    tokenKeys: string[];
    tokenPoints: Array<{
      label: string;
      total?: number;
      stacks: Array<{ key: string; value: number }>;
    }>;
  }>({
    modelKeys: [],
    modelPoints: [],
    tokenKeys: ["输入", "输出", "推理", "缓存"],
    tokenPoints: [],
  });

  const normalizedPendingApiFilter = useMemo(
    () => normalizeFilterValue(pendingApiFilter),
    [pendingApiFilter],
  );
  const normalizedPendingChannelFilter = useMemo(
    () => normalizeFilterList(pendingChannelFilter),
    [pendingChannelFilter],
  );
  const normalizedApiFilter = useMemo(() => normalizeFilterValue(apiFilter), [apiFilter]);
  const normalizedModelFilter = useMemo(() => normalizeFilterValue(modelFilter), [modelFilter]);
  const normalizedChannelFilter = useMemo(() => normalizeFilterList(channelFilter), [channelFilter]);

  const filterRequestKey = useMemo(
    () =>
      JSON.stringify({
        days: timeRange,
        apiKey: normalizedPendingApiFilter,
        channels: normalizedPendingChannelFilter,
      }),
    [normalizedPendingApiFilter, normalizedPendingChannelFilter, timeRange],
  );

  const summaryRequestKey = useMemo(
    () =>
      JSON.stringify({
        days: timeRange,
        apiKey: normalizedApiFilter,
        model: normalizedModelFilter,
        channels: normalizedChannelFilter,
      }),
    [normalizedApiFilter, normalizedChannelFilter, normalizedModelFilter, timeRange],
  );

  const clearCharts = useCallback(() => {
    setModelDistributionItems([]);
    setDailySeries([]);
    setHourlySeries({
      modelKeys: [],
      modelPoints: [],
      tokenKeys: ["输入", "输出", "推理", "缓存"],
      tokenPoints: [],
    });
  }, []);

  const refreshSummary = useCallback(async ({ force = false }: { force?: boolean } = {}) => {
    if (!force && lastAutoSummaryKeyRef.current === summaryRequestKey) {
      return;
    }

    const requestId = summaryRequestIdRef.current + 1;
    summaryRequestIdRef.current = requestId;
    chartRequestIdRef.current += 1;
    if (!force) {
      lastAutoSummaryKeyRef.current = summaryRequestKey;
    }

    setIsSummaryRefreshing(true);
    setError(null);
    setDeferredChartsRequest(null);
    setIsChartsRefreshing(false);
    try {
      const summaryRes = await (force
        ? usageApi.getMonitorSummary(
            timeRange,
            normalizedApiFilter,
            normalizedModelFilter,
            normalizedChannelFilter,
          )
        : getCachedMonitorRequest(monitorAutoRequestCache.summary, summaryRequestKey, () =>
            usageApi.getMonitorSummary(
              timeRange,
              normalizedApiFilter,
              normalizedModelFilter,
              normalizedChannelFilter,
            ),
          ));
      if (requestId !== summaryRequestIdRef.current) {
        return;
      }

      startTransition(() => {
        setSummary({
          requestCount: summaryRes.summary.TotalRequests,
          successCount: summaryRes.summary.SuccessRequests,
          failedCount: summaryRes.summary.FailedRequests,
          successRate: summaryRes.summary.SuccessRate,
          processedTokens: summaryRes.summary.ProcessedTokens,
          totalTokens: summaryRes.summary.TotalTokens,
          cachedTokens: summaryRes.summary.CachedTokens,
          inputTokens: summaryRes.summary.InputTokens,
          outputTokens: summaryRes.summary.OutputTokens,
        });
      });

      if (summaryRes.summary.TotalRequests > 0) {
        setDeferredChartsRequest({ key: summaryRequestKey, nonce: Date.now(), force });
      } else {
        clearCharts();
      }
    } catch (requestError) {
      if (requestId !== summaryRequestIdRef.current) {
        return;
      }
      const message = requestError instanceof Error ? requestError.message : "数据获取失败";
      setError(message);
    } finally {
      if (requestId === summaryRequestIdRef.current) {
        setIsSummaryRefreshing(false);
      }
    }
  }, [
    clearCharts,
    normalizedApiFilter,
    normalizedChannelFilter,
    normalizedModelFilter,
    summaryRequestKey,
    timeRange,
  ]);

  const refreshCharts = useCallback(
    async ({ key, force = false }: { key: string; force?: boolean }) => {
      if (!force && lastAutoChartKeyRef.current === key) {
        return;
      }

      const requestId = chartRequestIdRef.current + 1;
      chartRequestIdRef.current = requestId;
      if (!force) {
        lastAutoChartKeyRef.current = key;
      }

      setIsChartsRefreshing(true);
      try {
        const chartResponses = await (force
          ? Promise.all([
              usageApi.getMonitorModelDistribution(
                timeRange,
                10,
                normalizedApiFilter,
                normalizedModelFilter,
                normalizedChannelFilter,
              ),
              usageApi.getMonitorDailyTrend(
                timeRange,
                normalizedApiFilter,
                normalizedModelFilter,
                normalizedChannelFilter,
              ),
              usageApi.getMonitorHourly(
                24,
                normalizedApiFilter,
                normalizedModelFilter,
                normalizedChannelFilter,
              ),
            ]).then(([distributionRes, dailyRes, hourlyRes]) => ({
              distributionRes,
              dailyRes,
              hourlyRes,
            }))
          : getCachedMonitorRequest(monitorAutoRequestCache.charts, key, async () => {
              const [distributionRes, dailyRes, hourlyRes] = await Promise.all([
                usageApi.getMonitorModelDistribution(
                  timeRange,
                  10,
                  normalizedApiFilter,
                  normalizedModelFilter,
                  normalizedChannelFilter,
                ),
                usageApi.getMonitorDailyTrend(
                  timeRange,
                  normalizedApiFilter,
                  normalizedModelFilter,
                  normalizedChannelFilter,
                ),
                usageApi.getMonitorHourly(
                  24,
                  normalizedApiFilter,
                  normalizedModelFilter,
                  normalizedChannelFilter,
                ),
              ]);
              return { distributionRes, dailyRes, hourlyRes };
            }));

        if (requestId !== chartRequestIdRef.current) {
          return;
        }

        const { distributionRes, dailyRes, hourlyRes } = chartResponses;

        startTransition(() => {
          setModelDistributionItems(distributionRes.items);

          setDailySeries(
            dailyRes.items.map((item) => ({
              label: formatMonthDay(new Date(`${item.day}T00:00:00`)),
              requests: item.requests,
              inputTokens: item.input_tokens,
              outputTokens: item.output_tokens,
              totalTokens: item.total_tokens,
            })),
          );

          const hourWindow = 24;
          const now = Date.now();
          const endHour = Math.floor(now / 3_600_000);
          const startHour = endHour - hourWindow + 1;
          const hourLabels = Array.from({ length: hourWindow }).map((_, index) => {
            const hour = startHour + index;
            const date = new Date(hour * 3_600_000);
            const label = `${String(date.getHours()).padStart(2, "0")}:00`;
            return { hour, label };
          });

          const modelBuckets = new Map<number, Map<string, number>>();
          const tokenBuckets = new Map<
            number,
            {
              input: number;
              output: number;
              reasoning: number;
              cached: number;
              processed: number;
            }
          >();
          hourlyRes.items.forEach((item) => {
            const ts = new Date(item.hour).getTime();
            if (!Number.isFinite(ts)) return;
            const hour = Math.floor(ts / 3_600_000);
            if (hour < startHour || hour > endHour) return;

            const modelMap = modelBuckets.get(hour) ?? new Map<string, number>();
            modelMap.set(item.model, (modelMap.get(item.model) ?? 0) + item.requests);
            modelBuckets.set(hour, modelMap);

            const tokens = tokenBuckets.get(hour) ?? {
              input: 0,
              output: 0,
              reasoning: 0,
              cached: 0,
              processed: 0,
            };
            tokenBuckets.set(hour, {
              input: tokens.input + item.input_tokens,
              output: tokens.output + item.output_tokens,
              reasoning: tokens.reasoning + item.reasoning_tokens,
              cached: tokens.cached + item.cached_tokens,
              processed: tokens.processed + item.processed_tokens,
            });
          });

          const rankedModels = [...distributionRes.items]
            .sort(
              (left, right) =>
                right.requests - left.requests || left.model.localeCompare(right.model),
            )
            .slice(0, 5)
            .map((item) => item.model);
          const modelKeys = [...rankedModels, "其他"];

          const modelPoints = hourLabels.map(({ hour, label }) => {
            const map = modelBuckets.get(hour) ?? new Map<string, number>();
            const stacks = modelKeys.map((key) => {
              if (key === "其他") {
                const sum = [...map.entries()].reduce((acc, [model, value]) => {
                  return rankedModels.includes(model) ? acc : acc + value;
                }, 0);
                return { key, value: sum };
              }
              return { key, value: map.get(key) ?? 0 };
            });
            return { label, stacks, successRate: 0 };
          });

          const tokenPoints = hourLabels.map(({ hour, label }) => {
            const totals = tokenBuckets.get(hour) ?? {
              input: 0,
              output: 0,
              reasoning: 0,
              cached: 0,
              processed: 0,
            };
            return {
              label,
              total: totals.processed,
              stacks: [
                { key: "输入", value: totals.input },
                { key: "输出", value: totals.output },
                { key: "推理", value: totals.reasoning },
                { key: "缓存", value: totals.cached },
              ],
            };
          });

          setHourlySeries({
            modelKeys,
            modelPoints,
            tokenKeys: ["输入", "输出", "推理", "缓存"],
            tokenPoints,
          });
        });
      } catch (requestError) {
        if (requestId !== chartRequestIdRef.current) {
          return;
        }
        const message = requestError instanceof Error ? requestError.message : "图表数据获取失败";
        setError(message);
      } finally {
        if (requestId === chartRequestIdRef.current) {
          setIsChartsRefreshing(false);
        }
      }
    },
    [normalizedApiFilter, normalizedChannelFilter, normalizedModelFilter, timeRange],
  );

  const refreshData = useCallback(async () => {
    await refreshSummary({ force: true });
  }, [refreshSummary]);

  useEffect(() => {
    void refreshSummary();
  }, [refreshSummary]);

  useEffect(() => {
    if (!deferredChartsRequest) {
      return;
    }

    const timer = window.setTimeout(() => {
      void refreshCharts({ key: deferredChartsRequest.key, force: deferredChartsRequest.force });
    }, 0);

    return () => {
      window.clearTimeout(timer);
    };
  }, [deferredChartsRequest, refreshCharts]);

  const applyFilter = useCallback(() => {
    setApiFilter(normalizedPendingApiFilter);
    setModelFilter(normalizeFilterValue(pendingModelFilter));
    setChannelFilter(normalizedPendingChannelFilter);
  }, [normalizedPendingApiFilter, normalizedPendingChannelFilter, pendingModelFilter]);

  const fetchFilterOptions = useCallback(async () => {
    if (lastAutoFilterKeyRef.current === filterRequestKey) {
      return;
    }

    const requestId = filterRequestIdRef.current + 1;
    filterRequestIdRef.current = requestId;
    lastAutoFilterKeyRef.current = filterRequestKey;
    try {
      const response = await getCachedMonitorRequest(monitorAutoRequestCache.filters, filterRequestKey, () =>
        usageApi.getMonitorFilters(timeRange, normalizedPendingApiFilter || undefined, normalizedPendingChannelFilter),
      );
      if (requestId !== filterRequestIdRef.current) {
        return;
      }
      const normalized = response.filters ?? EMPTY_FILTER_OPTIONS;
      setFilterOptions(normalized);

      const modelExists = (value: string) => !value || normalized.models.includes(value);
      const validChannelValues = new Set(normalized.channel_options.map((item) => item.value));
      const normalizeChannelSelection = (value: string[]) =>
        value.filter((item) => validChannelValues.has(item));
      if (!modelExists(pendingModelFilter)) {
        setPendingModelFilter("");
      }
      const nextPendingChannelFilter = normalizeChannelSelection(normalizedPendingChannelFilter);
      if (nextPendingChannelFilter.length !== pendingChannelFilter.length) {
        setPendingChannelFilter(nextPendingChannelFilter);
      }
      if (normalizedPendingApiFilter === normalizedApiFilter && !modelExists(modelFilter)) {
        setModelFilter("");
      }
      if (normalizedPendingApiFilter === normalizedApiFilter) {
        const nextChannelFilter = normalizeChannelSelection(normalizedChannelFilter);
        if (nextChannelFilter.length !== channelFilter.length) {
          setChannelFilter(nextChannelFilter);
        }
      }
    } catch (requestError) {
      if (requestId !== filterRequestIdRef.current) {
        return;
      }
      const message = requestError instanceof Error ? requestError.message : "筛选项获取失败";
      notify({ type: "error", message });
    }
  }, [
    channelFilter.length,
    filterRequestKey,
    modelFilter,
    normalizedApiFilter,
    normalizedChannelFilter,
    normalizedPendingApiFilter,
    normalizedPendingChannelFilter,
    notify,
    pendingModelFilter,
    timeRange,
  ]);

  const toggleDailyLegend = useCallback((key: string) => {
    if (key !== "输入 Token" && key !== "输出 Token" && key !== "请求数") return;
    setDailyLegendSelected((prev) => ({
      ...prev,
      [key]: !(prev[key] ?? true),
    }));
  }, []);

  const toggleHourlyModelLegend = useCallback((key: string) => {
    setHourlyModelSelected((prev) => ({
      ...prev,
      [key]: !(prev[key] ?? true),
    }));
  }, []);

  const toggleHourlyTokenLegend = useCallback((key: string) => {
    setHourlyTokenSelected((prev) => ({
      ...prev,
      [key]: !(prev[key] ?? true),
    }));
  }, []);

  const hasData = summary.requestCount > 0;
  const isLoading = isSummaryRefreshing || isChartsRefreshing || isPending;

  useEffect(() => {
    void fetchFilterOptions();
  }, [fetchFilterOptions]);

  const keyOptions = useMemo(() => {
    const names = filterOptions.api_key_names ?? {};
    return [
      { value: "", label: "全部 Key" },
      ...filterOptions.api_keys.map((key) => ({
        value: key,
        label: names[key] || key,
        searchText: `${names[key] || ""} ${key}`,
      })),
    ];
  }, [filterOptions.api_key_names, filterOptions.api_keys]);

  const modelOptions = useMemo(() => {
    return [
      { value: "", label: "全部模型" },
      ...filterOptions.models.map((item) => ({
        value: item,
        label: item,
      })),
    ];
  }, [filterOptions.models]);

  const channelOptions = useMemo(() => {
    return filterOptions.channel_options;
  }, [filterOptions.channel_options]);

  const hourlyModelPalette = useMemo(() => {
    const palette = [
      "bg-emerald-400",
      "bg-violet-400",
      "bg-amber-400",
      "bg-pink-300",
      "bg-teal-400",
    ];
    const colorByKey: Record<string, string> = {};
    const classByKey: Record<string, string> = {};

    hourlySeries.modelKeys.forEach((key, index) => {
      if (key === "其他") {
        colorByKey[key] = "rgba(148,163,184,0.58)";
        classByKey[key] = "bg-slate-400";
        return;
      }
      colorByKey[key] = HOURLY_MODEL_COLORS[index % HOURLY_MODEL_COLORS.length];
      classByKey[key] = palette[index % palette.length] ?? "bg-slate-400";
    });

    colorByKey["总请求"] = "#3b82f6";
    classByKey["总请求"] = "bg-blue-500";

    return { colorByKey, classByKey };
  }, [hourlySeries.modelKeys]);

  const hourlyTokenPalette = useMemo(() => {
    return {
      colorByKey: {
        输入: "rgba(110,231,183,0.88)",
        输出: "rgba(196,181,253,0.88)",
        推理: "rgba(252,211,77,0.88)",
        缓存: "rgba(94,234,212,0.88)",
        "处理量 Token": "#3b82f6",
      } as Record<string, string>,
      classByKey: {
        输入: "bg-emerald-400",
        输出: "bg-violet-400",
        推理: "bg-amber-400",
        缓存: "bg-teal-400",
        "处理量 Token": "bg-blue-500",
      } as Record<string, string>,
    };
  }, []);

  useEffect(() => {
    setHourlyModelSelected((prev) => {
      const next = { ...prev };
      for (const key of hourlySeries.modelKeys) {
        if (!(key in next)) next[key] = true;
      }
      if (!("总请求" in next)) next["总请求"] = true;
      return next;
    });
  }, [hourlySeries.modelKeys]);

  useEffect(() => {
    setHourlyTokenSelected((prev) => {
      const next = { ...prev };
      for (const key of hourlySeries.tokenKeys) {
        if (!(key in next)) next[key] = true;
      }
      if (!("处理量 Token" in next)) next["处理量 Token"] = true;
      return next;
    });
  }, [hourlySeries.tokenKeys]);

  const modelDistributionData = useMemo(() => {
    const sortedModels = [...modelDistributionItems].sort((left, right) => {
      const leftValue = modelMetric === "requests" ? left.requests : left.processed_tokens;
      const rightValue = modelMetric === "requests" ? right.requests : right.processed_tokens;
      return rightValue - leftValue || left.model.localeCompare(right.model);
    });
    const top = sortedModels.slice(0, 10);
    const otherValue = sortedModels.slice(10).reduce((acc, item) => {
      return acc + (modelMetric === "requests" ? item.requests : item.processed_tokens);
    }, 0);
    const nextModelDistribution = top.map((item) => ({
      name: item.model,
      value: modelMetric === "requests" ? item.requests : item.processed_tokens,
      requests: item.requests,
      totalTokens: item.total_tokens,
      cachedTokens: item.cached_tokens,
      processedTokens: item.processed_tokens,
    }));
    if (otherValue > 0) {
      nextModelDistribution.push({
        name: "其他",
        value: otherValue,
        requests: 0,
        totalTokens: 0,
        cachedTokens: 0,
        processedTokens: 0,
      });
    }
    return nextModelDistribution;
  }, [modelDistributionItems, modelMetric]);

  const modelDistributionOption = useMemo(
    () => createModelDistributionOption({ isDark, data: modelDistributionData }),
    [isDark, modelDistributionData],
  );

  const dailyLegendAvailability = useMemo(() => {
    const points = dailySeries.filter(
      (item) => item.requests > 0 || item.inputTokens > 0 || item.outputTokens > 0,
    );
    const visiblePoints = points.length > 0 ? points : dailySeries;
    const requestY = visiblePoints.map((item) => item.requests);
    const inputY = visiblePoints.map((item) => item.inputTokens);
    const outputY = visiblePoints.map((item) => item.outputTokens);

    return {
      hasInput: inputY.some((value) => value > 0),
      hasOutput: outputY.some((value) => value > 0),
      hasRequests: requestY.some((value) => value > 0),
    };
  }, [dailySeries]);

  const modelDistributionLegend = useMemo(() => {
    const total = modelDistributionData.reduce(
      (acc, item) => acc + (Number.isFinite(item.value) ? item.value : 0),
      0,
    );

    return modelDistributionData.map((item, index) => {
      const colorClass =
        index < CHART_COLOR_CLASSES.length ? CHART_COLOR_CLASSES[index] : "bg-slate-400";
      const value = Number(item.value ?? 0);
      const percent = total > 0 ? (value / total) * 100 : 0;

      return {
        name: item.name,
        valueLabel: formatCompact(value),
        percentLabel: `${percent.toFixed(1)}%`,
        colorClass,
      };
    });
  }, [modelDistributionData]);

  const dailyTrendOption = useMemo(
    () => createDailyTrendOption({ dailySeries, dailyLegendSelected, isDark }),
    [dailyLegendSelected, dailySeries, isDark, timeRange],
  );

  const hourlyModelOption = useMemo(
    () =>
      createHourlyModelOption({
        hourlySeries,
        modelHourWindow,
        hourlyModelSelected,
        paletteColorByKey: hourlyModelPalette.colorByKey,
        isDark,
      }),
    [
      hourlyModelPalette.colorByKey,
      hourlyModelSelected,
      hourlySeries.modelKeys,
      hourlySeries.modelPoints,
      isDark,
      modelHourWindow,
    ],
  );

  const hourlyTokenOption = useMemo(
    () =>
      createHourlyTokenOption({
        hourlySeries,
        tokenHourWindow,
        hourlyTokenSelected,
        paletteColorByKey: hourlyTokenPalette.colorByKey,
        isDark,
      }),
    [
      hourlySeries.tokenKeys,
      hourlySeries.tokenPoints,
      hourlyTokenPalette.colorByKey,
      hourlyTokenSelected,
      isDark,
      tokenHourWindow,
    ],
  );

  const modelActions = (
    <Tabs
      value={modelMetric}
      onValueChange={(next) => setModelMetric(next as "requests" | "processed")}
    >
      <TabsList>
        <TabsTrigger value="requests">请求</TabsTrigger>
        <TabsTrigger value="processed">处理量</TabsTrigger>
      </TabsList>
    </Tabs>
  );

  return (
    <div className="space-y-4">
      <section className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-neutral-800 dark:bg-neutral-950/70">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <h2 className="flex items-center gap-2 text-lg font-semibold text-slate-900 dark:text-white">
              <ChartSpline size={18} className="text-slate-900 dark:text-white" />
              <span>监控中心</span>
            </h2>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <TimeRangeSelector value={timeRange} onChange={setTimeRange} />
            <SearchableSelect
              value={pendingApiFilter}
              onChange={setPendingApiFilter}
              options={keyOptions}
              placeholder="全部 Key"
              searchPlaceholder="搜索 Key…"
              aria-label="按 Key 名称过滤"
              className="min-w-[180px] justify-between"
            />
            <SearchableSelect
              value={pendingModelFilter}
              onChange={setPendingModelFilter}
              options={modelOptions}
              placeholder="全部模型"
              searchPlaceholder="搜索模型…"
              aria-label="按模型过滤"
              className="min-w-[200px] justify-between"
            />
            <MultiSelect
              value={pendingChannelFilter}
              onChange={setPendingChannelFilter}
              options={channelOptions}
              placeholder="全部渠道"
              emptyLabel="全部渠道"
              selectAllLabel="全部渠道"
              searchPlaceholder="搜索渠道…"
              emptyResultLabel="无匹配渠道"
              className="min-w-[240px]"
              maxVisibleTags={1}
            />
            <button
              type="button"
              onClick={applyFilter}
              className="inline-flex items-center gap-1.5 rounded-2xl border border-slate-200 bg-white px-3 py-1.5 text-sm font-semibold text-slate-700 transition hover:bg-slate-50 dark:border-neutral-800 dark:bg-neutral-950/60 dark:text-white/80 dark:hover:bg-white/10"
            >
              <Filter size={14} />
              应用过滤
            </button>
            <button
              type="button"
              onClick={() => void refreshData()}
              disabled={isLoading}
              aria-busy={isLoading}
              className="inline-flex min-w-[96px] items-center justify-center gap-1.5 rounded-2xl bg-slate-900 px-3 py-1.5 text-sm font-semibold text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-70 dark:bg-white dark:text-neutral-950 dark:hover:bg-slate-200"
            >
              <RefreshCw size={14} className={isLoading ? "animate-spin" : ""} />
              <span className="grid">
                <span
                  className={
                    isLoading
                      ? "col-start-1 row-start-1 opacity-0"
                      : "col-start-1 row-start-1 opacity-100"
                  }
                >
                  刷新
                </span>
                <span
                  className={
                    isLoading
                      ? "col-start-1 row-start-1 opacity-100"
                      : "col-start-1 row-start-1 opacity-0"
                  }
                >
                  刷新中
                </span>
              </span>
            </button>
          </div>
        </div>

        {error ? (
          <div className="mt-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            {error}
          </div>
        ) : null}
      </section>

      <Reveal>
        <section className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <KpiCard
            title="总请求"
            value={<AnimatedNumber value={summary.requestCount} format={formatNumber} />}
            hint="已按时间范围过滤"
            icon={Activity}
          />
          <KpiCard
            title="成功率"
            value={<AnimatedNumber value={summary.successRate} format={formatRate} />}
            hint={`成功 ${formatNumber(summary.successCount)} / 失败 ${formatNumber(summary.failedCount)}`}
            icon={ShieldCheck}
          />
          <KpiCard
            title="处理量 Token"
            value={<AnimatedNumber value={summary.processedTokens} format={formatNumber} />}
            hint={`账面 ${formatNumber(summary.totalTokens)} · 缓存 ${formatNumber(summary.cachedTokens)}`}
            icon={Sigma}
          />
          <KpiCard
            title="缓存 Token"
            value={<AnimatedNumber value={summary.cachedTokens} format={formatNumber} />}
            hint={`输入 ${formatNumber(summary.inputTokens)} · 输出 ${formatNumber(summary.outputTokens)}`}
            icon={Coins}
          />
        </section>
      </Reveal>

      {!hasData && !isLoading ? (
        <Reveal>
          <section className="rounded-2xl border border-dashed border-slate-200 bg-white p-10 text-center shadow-sm dark:border-neutral-800 dark:bg-neutral-950/60">
            <div className="mx-auto flex max-w-md flex-col items-center gap-3">
              <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-slate-900/5 text-slate-700 dark:bg-white/10 dark:text-white/70">
                <ChartSpline size={20} />
              </div>
              <p className="text-sm font-semibold text-slate-900 dark:text-white">暂无监控数据</p>
              <p className="text-sm text-slate-600 dark:text-white/65">
                可以点击上方“刷新”重新拉取数据。
              </p>
              <button
                type="button"
                onClick={() => void refreshData()}
                className="inline-flex min-w-[96px] items-center justify-center gap-1.5 rounded-2xl bg-slate-900 px-3 py-1.5 text-sm font-semibold text-white transition hover:bg-slate-800 dark:bg-white dark:text-neutral-950 dark:hover:bg-slate-200"
              >
                <RefreshCw size={14} />
                刷新
              </button>
            </div>
          </section>
        </Reveal>
      ) : (
        <>
          <Reveal>
            <section className="grid gap-4 lg:grid-cols-[minmax(0,560px)_minmax(0,1fr)]">
              <Card
                title="模型用量分布"
                description={`最近 ${timeRange} 天 · 按${modelMetric === "requests" ? "请求数" : "处理量 Token"} · Top10`}
                actions={modelActions}
                loading={isChartsRefreshing}
              >
                <div className="grid h-72 grid-cols-[minmax(0,1fr)_220px] gap-4">
                  <EChart option={modelDistributionOption} className="h-72 min-w-0" />
                  <div className="flex h-72 flex-col justify-center gap-2 overflow-y-auto pr-1">
                    {modelDistributionLegend.map((item) => (
                      <div
                        key={item.name}
                        className="grid grid-cols-[minmax(0,120px)_40px_52px] items-center gap-x-1 text-sm"
                      >
                        <div className="flex min-w-0 items-center gap-2">
                          <span
                            className={`h-3.5 w-3.5 shrink-0 rounded-full ${item.colorClass} opacity-80 ring-1 ring-black/5 dark:ring-white/10`}
                          />
                          <span className="min-w-0 truncate text-slate-700 dark:text-white/80">
                            {item.name}
                          </span>
                        </div>
                        <span className="text-right font-semibold tabular-nums text-slate-900 dark:text-white">
                          {item.valueLabel}
                        </span>
                        <span className="text-right tabular-nums text-slate-500 dark:text-white/55">
                          {item.percentLabel}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              </Card>

              <Card
                title="每日用量趋势"
                description={`最近 ${timeRange} 天 · 请求数与 Token 用量趋势`}
                loading={isChartsRefreshing}
              >
                <div className="flex h-72 min-w-0 flex-col overflow-hidden">
                  <EChart
                    option={dailyTrendOption}
                    className="min-h-0 flex-1 min-w-0"
                    replaceMerge="series"
                  />
                  <ChartLegend
                    className="shrink-0 pt-4"
                    items={[
                      ...(dailyLegendAvailability.hasInput
                        ? [
                            {
                              key: "输入 Token",
                              label: "输入 Token",
                              colorClass: "bg-violet-400",
                              enabled: dailyLegendSelected["输入 Token"] ?? true,
                              onToggle: toggleDailyLegend,
                            },
                          ]
                        : []),
                      ...(dailyLegendAvailability.hasOutput
                        ? [
                            {
                              key: "输出 Token",
                              label: "输出 Token",
                              colorClass: "bg-emerald-400",
                              enabled: dailyLegendSelected["输出 Token"] ?? true,
                              onToggle: toggleDailyLegend,
                            },
                          ]
                        : []),
                      ...(dailyLegendAvailability.hasRequests
                        ? [
                            {
                              key: "请求数",
                              label: "请求数",
                              colorClass: "bg-blue-500",
                              enabled: dailyLegendSelected["请求数"] ?? true,
                              onToggle: toggleDailyLegend,
                            },
                          ]
                        : []),
                    ]}
                  />
                </div>
              </Card>
            </section>
          </Reveal>

          <Reveal>
              <Card
                title="每小时模型请求分布"
                description="按小时聚合（Top5 模型 + 其他）"
                actions={<HourWindowSelector value={modelHourWindow} onChange={setModelHourWindow} />}
                loading={isChartsRefreshing}
              >
              <div className="flex h-72 flex-col overflow-hidden">
                <EChart
                  option={hourlyModelOption}
                  className="min-h-0 flex-1"
                  replaceMerge="series"
                />
                <ChartLegend
                  className="shrink-0 pt-4"
                  items={[
                    ...hourlySeries.modelKeys.map((key) => ({
                      key,
                      label: key,
                      colorClass: hourlyModelPalette.classByKey[key] ?? "bg-slate-400",
                      enabled: hourlyModelSelected[key] ?? true,
                      onToggle: toggleHourlyModelLegend,
                    })),
                    {
                      key: "总请求",
                      label: "总请求",
                      colorClass: hourlyModelPalette.classByKey["总请求"] ?? "bg-blue-500",
                      enabled: hourlyModelSelected["总请求"] ?? true,
                      onToggle: toggleHourlyModelLegend,
                    },
                  ]}
                />
              </div>
            </Card>
          </Reveal>

          <Reveal>
              <Card
                title="每小时 Token 用量"
                description="按小时聚合（输入 / 输出 / 推理 / 缓存，处理量按 provider 口径归一）"
                actions={<HourWindowSelector value={tokenHourWindow} onChange={setTokenHourWindow} />}
                loading={isChartsRefreshing}
              >
              <div className="flex h-72 flex-col overflow-hidden">
                <EChart
                  option={hourlyTokenOption}
                  className="min-h-0 flex-1"
                  replaceMerge="series"
                />
                <ChartLegend
                  className="shrink-0 pt-4"
                  items={[
                    ...hourlySeries.tokenKeys.map((key) => ({
                      key,
                      label: key,
                      colorClass: hourlyTokenPalette.classByKey[key] ?? "bg-slate-400",
                      enabled: hourlyTokenSelected[key] ?? true,
                      onToggle: toggleHourlyTokenLegend,
                    })),
                    {
                      key: "处理量 Token",
                      label: "处理量 Token",
                      colorClass: hourlyTokenPalette.classByKey["处理量 Token"] ?? "bg-blue-500",
                      enabled: hourlyTokenSelected["处理量 Token"] ?? true,
                      onToggle: toggleHourlyTokenLegend,
                    },
                  ]}
                />
              </div>
            </Card>
          </Reveal>
        </>
      )}
    </div>
  );
}
