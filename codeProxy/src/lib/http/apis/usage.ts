import { apiClient } from "@/lib/http/client";

function appendMultiValueQuery(
  query: URLSearchParams,
  key: string,
  values?: string[],
) {
  values
    ?.map((value) => value.trim())
    .filter(Boolean)
    .forEach((value) => query.append(key, value));
}

export interface ChannelOption {
  value: string;
  label: string;
}

export interface UsageFilterOptions {
  api_keys: string[];
  api_key_names: Record<string, string>;
  models: string[];
  channels: string[];
  channel_options: ChannelOption[];
}

function uniqueTrimmedStrings(values: string[]): string[] {
  const seen = new Set<string>();
  const result: string[] = [];
  values.forEach((value) => {
    const trimmed = value.trim();
    if (!trimmed || seen.has(trimmed)) return;
    seen.add(trimmed);
    result.push(trimmed);
  });
  return result;
}

function normalizeChannelOptions(
  rawOptions: unknown,
  fallbackChannels: string[],
): ChannelOption[] {
  const options: ChannelOption[] = [];
  const seen = new Set<string>();

  if (Array.isArray(rawOptions)) {
    rawOptions.forEach((item) => {
      if (!item || typeof item !== "object") return;
      const value =
        typeof (item as { value?: unknown }).value === "string"
          ? (item as { value: string }).value.trim()
          : "";
      const label =
        typeof (item as { label?: unknown }).label === "string"
          ? (item as { label: string }).label.trim()
          : "";
      if (!value || !label || seen.has(value)) return;
      seen.add(value);
      options.push({ value, label });
    });
  }

  if (options.length > 0) {
    return options;
  }

  return uniqueTrimmedStrings(fallbackChannels).map((channel) => ({
    value: channel,
    label: channel,
  }));
}

function normalizeFilterOptions(rawFilters: unknown): UsageFilterOptions {
  const source =
    rawFilters && typeof rawFilters === "object"
      ? (rawFilters as Partial<UsageFilterOptions>)
      : undefined;
  const apiKeys = Array.isArray(source?.api_keys)
    ? source.api_keys.filter((item): item is string => typeof item === "string")
    : [];
  const models = Array.isArray(source?.models)
    ? source.models.filter((item): item is string => typeof item === "string")
    : [];
  const channels = Array.isArray(source?.channels)
    ? source.channels.filter((item): item is string => typeof item === "string")
    : [];
  const channelOptions = normalizeChannelOptions(
    source?.channel_options,
    channels,
  );

  return {
    api_keys: apiKeys,
    api_key_names:
      source?.api_key_names && typeof source.api_key_names === "object"
        ? source.api_key_names
        : {},
    models,
    channels:
      channels.length > 0
        ? uniqueTrimmedStrings(channels)
        : uniqueTrimmedStrings(channelOptions.map((item) => item.label)),
    channel_options: channelOptions,
  };
}

export interface UsageExportPayload {
  version?: number;
  exported_at?: string;
  usage?: Record<string, unknown>;
  [key: string]: unknown;
}

export interface UsageImportResponse {
  added?: number;
  skipped?: number;
  total_requests?: number;
  failed_requests?: number;
  [key: string]: unknown;
}

export interface UsageOverviewResponse {
  days: number;
  summary: {
    TotalRequests: number;
    SuccessRequests: number;
    FailedRequests: number;
    SuccessRate: number;
    InputTokens: number;
    OutputTokens: number;
    ReasoningTokens: number;
    CachedTokens: number;
    TotalTokens: number;
  };
  request_trend: Array<{
    bucket: string;
    requests: number;
    input_tokens: number;
    output_tokens: number;
    reasoning_tokens: number;
    cached_tokens: number;
    total_tokens: number;
  }>;
  token_breakdown: Array<{
    bucket: string;
    requests: number;
    input_tokens: number;
    output_tokens: number;
    reasoning_tokens: number;
    cached_tokens: number;
    total_tokens: number;
  }>;
  apis: Array<{
    endpoint: string;
    total_requests: number;
    success_count: number;
    failure_count: number;
    total_tokens: number;
    models: Array<{
      model: string;
      requests: number;
      success_count: number;
      failure_count: number;
      total_tokens: number;
    }>;
  }>;
  models: Array<{
    model: string;
    requests: number;
    success_count: number;
    failure_count: number;
    total_tokens: number;
  }>;
  credentials: Array<{
    source: string;
    auth_index: string;
    requests: number;
    success_count: number;
    failure_count: number;
  }>;
  service_health: Array<{
    bucket: string;
    success_count: number;
    failure_count: number;
  }>;
}

export interface UsageCredentialHealthResponse {
  days: number;
  items: Array<{
    auth_index: string;
    source: string;
    bucket: string;
    success_count: number;
    failure_count: number;
  }>;
}

export interface UsageModelStatsResponse {
  days: number;
  items: Array<{
    model: string;
    requests: number;
    success_count: number;
    failure_count: number;
    input_tokens: number;
    output_tokens: number;
    reasoning_tokens: number;
    cached_tokens: number;
    total_tokens: number;
    last_used_at: string;
  }>;
}

export interface UsageSourceStatsResponse {
  days: number;
  recent_minutes: number;
  block_minutes: number;
  items: Array<{
    source: string;
    auth_index: string;
    success_count: number;
    failure_count: number;
    blocks: Array<{
      success_count: number;
      failure_count: number;
    }>;
  }>;
}

export const usageApi = {
  async getUsageLogs(params: {
    page?: number;
    size?: number;
    days?: number;
    api_key?: string;
    model?: string;
    status?: string;
    channel_name?: string[];
  }): Promise<UsageLogsResponse> {
    const qs = new URLSearchParams();
    if (params.page) qs.set("page", String(params.page));
    if (params.size) qs.set("size", String(params.size));
    if (params.days) qs.set("days", String(params.days));
    if (params.api_key) qs.set("api_key", params.api_key);
    if (params.model) qs.set("model", params.model);
    if (params.status) qs.set("status", params.status);
    appendMultiValueQuery(qs, "channel_name", params.channel_name);
    const query = qs.toString();
    const response = await apiClient.get<
      Partial<UsageLogsResponse> | undefined
    >(`/usage/logs${query ? `?${query}` : ""}`);

    const rawItems = Array.isArray(response?.items) ? response.items : [];
    const rawStats = response?.stats;

    return {
      items: rawItems,
      total: typeof response?.total === "number" ? response.total : 0,
      page:
        typeof response?.page === "number" ? response.page : (params.page ?? 1),
      size:
        typeof response?.size === "number"
          ? response.size
          : (params.size ?? 50),
      filters: normalizeFilterOptions(response?.filters),
      stats: {
        total: typeof rawStats?.total === "number" ? rawStats.total : 0,
        success_rate:
          typeof rawStats?.success_rate === "number"
            ? rawStats.success_rate
            : 0,
        total_tokens:
          typeof rawStats?.total_tokens === "number"
            ? rawStats.total_tokens
            : 0,
      },
    };
  },

  exportUsage(): Promise<UsageExportPayload> {
    return apiClient.get<UsageExportPayload>("/usage/export");
  },

  importUsage(payload: unknown): Promise<UsageImportResponse> {
    return apiClient.post<UsageImportResponse>("/usage/import", payload);
  },

  getUsageOverview(days = 30, apiKey?: string): Promise<UsageOverviewResponse> {
    const query = new URLSearchParams({ days: String(days) });
    if (apiKey?.trim()) query.set("api_key", apiKey.trim());
    return apiClient.get<UsageOverviewResponse>(
      `/usage/overview?${query.toString()}`,
    );
  },

  getUsageCredentialHealth(
    days = 30,
    apiKey?: string,
  ): Promise<UsageCredentialHealthResponse> {
    const query = new URLSearchParams({ days: String(days) });
    if (apiKey?.trim()) query.set("api_key", apiKey.trim());
    return apiClient.get<UsageCredentialHealthResponse>(
      `/usage/credential-health?${query.toString()}`,
    );
  },

  getUsageModelStats(days = 30, limit = 500): Promise<UsageModelStatsResponse> {
    const query = new URLSearchParams({
      days: String(days),
      limit: String(limit),
    });
    return apiClient.get<UsageModelStatsResponse>(
      `/usage/models/stats?${query.toString()}`,
    );
  },

  getUsageSourceStats(
    days = 30,
    recentMinutes = 200,
    blockMinutes = 10,
  ): Promise<UsageSourceStatsResponse> {
    const query = new URLSearchParams({
      days: String(days),
      recent_minutes: String(recentMinutes),
      block_minutes: String(blockMinutes),
    });
    return apiClient.get<UsageSourceStatsResponse>(
      `/usage/source-stats?${query.toString()}`,
    );
  },

  getDashboardSummary(days = 7): Promise<DashboardSummary> {
    return apiClient.get<DashboardSummary>(`/dashboard-summary?days=${days}`);
  },

  getMonitorFilters(
    days = 7,
    apiKey?: string,
    channelNames?: string[],
  ): Promise<MonitorFiltersResponse> {
    const query = new URLSearchParams({ days: String(days) });
    if (apiKey?.trim()) query.set("api_key", apiKey.trim());
    appendMultiValueQuery(query, "channel_name", channelNames);
    return apiClient
      .get<Partial<MonitorFiltersResponse> | undefined>(
        `/monitor/filters?${query.toString()}`,
      )
      .then((response) => ({
        days: typeof response?.days === "number" ? response.days : days,
        filters: normalizeFilterOptions(response?.filters),
      }));
  },

  getMonitorSummary(
    days = 7,
    apiKey?: string,
    model?: string,
    channelNames?: string[],
  ): Promise<MonitorSummaryResponse> {
    const query = new URLSearchParams({ days: String(days) });
    if (apiKey?.trim()) query.set("api_key", apiKey.trim());
    if (model?.trim()) query.set("model", model.trim());
    appendMultiValueQuery(query, "channel_name", channelNames);
    return apiClient.get<MonitorSummaryResponse>(
      `/monitor/summary?${query.toString()}`,
    );
  },

  getMonitorModelDistribution(
    days = 7,
    limit = 10,
    apiKey?: string,
    model?: string,
    channelNames?: string[],
  ): Promise<MonitorModelDistributionResponse> {
    const query = new URLSearchParams({
      days: String(days),
      limit: String(limit),
    });
    if (apiKey?.trim()) query.set("api_key", apiKey.trim());
    if (model?.trim()) query.set("model", model.trim());
    appendMultiValueQuery(query, "channel_name", channelNames);
    return apiClient.get<MonitorModelDistributionResponse>(
      `/monitor/model-distribution?${query.toString()}`,
    );
  },

  getMonitorDailyTrend(
    days = 7,
    apiKey?: string,
    model?: string,
    channelNames?: string[],
  ): Promise<MonitorDailyTrendResponse> {
    const query = new URLSearchParams({ days: String(days) });
    if (apiKey?.trim()) query.set("api_key", apiKey.trim());
    if (model?.trim()) query.set("model", model.trim());
    appendMultiValueQuery(query, "channel_name", channelNames);
    return apiClient.get<MonitorDailyTrendResponse>(
      `/monitor/daily-trend?${query.toString()}`,
    );
  },

  getMonitorHourly(
    hours = 24,
    apiKey?: string,
    model?: string,
    channelNames?: string[],
  ): Promise<MonitorHourlyResponse> {
    const query = new URLSearchParams({ hours: String(hours) });
    if (apiKey?.trim()) query.set("api_key", apiKey.trim());
    if (model?.trim()) query.set("model", model.trim());
    appendMultiValueQuery(query, "channel_name", channelNames);
    return apiClient.get<MonitorHourlyResponse>(
      `/monitor/hourly?${query.toString()}`,
    );
  },

  getMonitorChannelStats(
    days = 7,
    limit = 10,
    apiKey?: string,
    model?: string,
    channelNames?: string[],
  ): Promise<MonitorChannelStatsResponse> {
    const query = new URLSearchParams({
      days: String(days),
      limit: String(limit),
    });
    if (apiKey?.trim()) query.set("api_key", apiKey.trim());
    if (model?.trim()) query.set("model", model.trim());
    appendMultiValueQuery(query, "channel_name", channelNames);
    return apiClient.get<MonitorChannelStatsResponse>(
      `/monitor/channel-stats?${query.toString()}`,
    );
  },

  getMonitorFailureStats(
    days = 7,
    limit = 10,
    apiKey?: string,
    model?: string,
    channelNames?: string[],
  ): Promise<MonitorFailureStatsResponse> {
    const query = new URLSearchParams({
      days: String(days),
      limit: String(limit),
    });
    if (apiKey?.trim()) query.set("api_key", apiKey.trim());
    if (model?.trim()) query.set("model", model.trim());
    appendMultiValueQuery(query, "channel_name", channelNames);
    return apiClient.get<MonitorFailureStatsResponse>(
      `/monitor/failure-stats?${query.toString()}`,
    );
  },

  async getLogContent(id: number): Promise<LogContentResponse> {
    return apiClient.get<LogContentResponse>(`/usage/logs/${id}/content`);
  },
};

export interface DashboardSummary {
  kpi: {
    total_requests: number;
    success_requests: number;
    failed_requests: number;
    success_rate: number;
    input_tokens: number;
    output_tokens: number;
    reasoning_tokens: number;
    cached_tokens: number;
    total_tokens: number;
  };
  counts: {
    api_keys: number;
    providers_total: number;
    gemini_keys: number;
    claude_keys: number;
    codex_keys: number;
    vertex_keys: number;
    openai_providers: number;
    auth_files: number;
  };
  days: number;
}

export interface MonitorSummaryResponse {
  days: number;
  summary: {
    TotalRequests: number;
    SuccessRequests: number;
    FailedRequests: number;
    SuccessRate: number;
    InputTokens: number;
    OutputTokens: number;
    ReasoningTokens: number;
    CachedTokens: number;
    TotalTokens: number;
  };
}

export interface MonitorFiltersResponse {
  days: number;
  filters: UsageFilterOptions;
}

export interface MonitorModelDistributionResponse {
  days: number;
  items: Array<{
    model: string;
    requests: number;
    tokens: number;
  }>;
}

export interface MonitorDailyTrendResponse {
  days: number;
  items: Array<{
    day: string;
    requests: number;
    input_tokens: number;
    output_tokens: number;
    reasoning_tokens: number;
    cached_tokens: number;
    total_tokens: number;
  }>;
}

export interface MonitorHourlyResponse {
  hours: number;
  items: Array<{
    hour: string;
    model: string;
    requests: number;
    input_tokens: number;
    output_tokens: number;
    reasoning_tokens: number;
    cached_tokens: number;
    total_tokens: number;
  }>;
}

export interface MonitorChannelStatsResponse {
  days: number;
  channels: Array<{
    source: string;
    requests: number;
    success_requests: number;
    failed_requests: number;
    success_rate: number;
    last_request_at: string;
  }>;
  models: Array<{
    source: string;
    model: string;
    requests: number;
    success_requests: number;
    failed_requests: number;
    success_rate: number;
    last_request_at: string;
  }>;
}

export interface MonitorFailureStatsResponse {
  days: number;
  channels: Array<{
    source: string;
    failed_requests: number;
    last_failed_at: string;
  }>;
  models: Array<{
    source: string;
    model: string;
    requests: number;
    success_requests: number;
    failed_requests: number;
    success_rate: number;
    last_request_at: string;
  }>;
}

export interface UsageLogItem {
  id: number;
  timestamp: string;
  api_key: string;
  api_key_name: string;
  model: string;
  reasoning_effort?: string;
  source: string;
  channel_name: string;
  auth_index: string;
  failed: boolean;
  latency_ms: number;
  input_tokens: number;
  output_tokens: number;
  reasoning_tokens: number;
  cached_tokens: number;
  total_tokens: number;
  has_content: boolean;
}

export interface UsageLogsResponse {
  items: UsageLogItem[];
  total: number;
  page: number;
  size: number;
  filters: UsageFilterOptions;
  stats: {
    total: number;
    success_rate: number;
    total_tokens: number;
  };
}

export interface LogContentResponse {
  id: number;
  input_content: string;
  output_content: string;
  model: string;
  request_meta?: Record<string, unknown>;
}
