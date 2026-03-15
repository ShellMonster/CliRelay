import { apiClient } from "@/lib/http/client";
import { normalizeConfigResponse } from "@/lib/http/transformers";
import type { Config } from "@/types";

export interface UserAgentRoutingProviderOption {
  id: string;
  label: string;
}

export interface UserAgentRoutingModelOption {
  id: string;
  label: string;
}

export interface UserAgentRoutingOptions {
  providers: UserAgentRoutingProviderOption[];
  models: UserAgentRoutingModelOption[];
}

export const configApi = {
  async getConfig(): Promise<Config> {
    const raw = await apiClient.get<Record<string, unknown>>("/config");
    return normalizeConfigResponse(raw);
  },

  updateDebug: (enabled: boolean) => apiClient.put("/debug", { value: enabled }),
  updateProxyUrl: (proxyUrl: string) => apiClient.put("/proxy-url", { value: proxyUrl }),
  clearProxyUrl: () => apiClient.delete("/proxy-url"),
  updateRequestRetry: (retryCount: number) =>
    apiClient.put("/request-retry", { value: retryCount }),
  updateSwitchProject: (enabled: boolean) =>
    apiClient.put("/quota-exceeded/switch-project", { value: enabled }),
  updateSwitchPreviewModel: (enabled: boolean) =>
    apiClient.put("/quota-exceeded/switch-preview-model", { value: enabled }),
  updateUsageStatistics: (enabled: boolean) =>
    apiClient.put("/usage-statistics-enabled", { value: enabled }),
  updateUsageLogContent: (enabled: boolean) =>
    apiClient.put("/usage-log-content-enabled", { value: enabled }),
  updateRequestLog: (enabled: boolean) => apiClient.put("/request-log", { value: enabled }),
  updateLoggingToFile: (enabled: boolean) => apiClient.put("/logging-to-file", { value: enabled }),
  getLogsMaxTotalSizeMb: async (): Promise<number> => {
    const data = await apiClient.get<Record<string, unknown>>("/logs-max-total-size-mb");
    const value = data?.["logs-max-total-size-mb"] ?? data?.logsMaxTotalSizeMb ?? 0;
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : 0;
  },
  updateLogsMaxTotalSizeMb: (value: number) => apiClient.put("/logs-max-total-size-mb", { value }),
  updateWsAuth: (enabled: boolean) => apiClient.put("/ws-auth", { value: enabled }),
  getForceModelPrefix: async (): Promise<boolean> => {
    const data = await apiClient.get<Record<string, unknown>>("/force-model-prefix");
    return Boolean(data?.["force-model-prefix"] ?? data?.forceModelPrefix ?? false);
  },
  updateForceModelPrefix: (enabled: boolean) =>
    apiClient.put("/force-model-prefix", { value: enabled }),
  getRoutingStrategy: async (): Promise<string> => {
    const data = await apiClient.get<Record<string, unknown>>("/routing/strategy");
    const strategy = data?.strategy ?? data?.["routing-strategy"] ?? data?.routingStrategy;
    return typeof strategy === "string" && strategy.trim() ? strategy.trim() : "round-robin";
  },
  updateRoutingStrategy: (strategy: string) =>
    apiClient.put("/routing/strategy", { value: strategy }),
  getUserAgentRoutingOptions: async (): Promise<UserAgentRoutingOptions> => {
    const data = await apiClient.get<Record<string, unknown>>("/routing/user-agent-options");
    const providersRaw = Array.isArray(data?.providers) ? data.providers : [];
    const modelsRaw = Array.isArray(data?.models) ? data.models : [];

    const providers = providersRaw
      .map((item) => {
        if (!item || typeof item !== "object") return null;
        const record = item as Record<string, unknown>;
        const id = String(record.id ?? "").trim();
        if (!id) return null;
        const label = String(record.label ?? id).trim() || id;
        return { id, label } satisfies UserAgentRoutingProviderOption;
      })
      .filter(Boolean) as UserAgentRoutingProviderOption[];

    const models = modelsRaw
      .map((item) => {
        if (!item || typeof item !== "object") return null;
        const record = item as Record<string, unknown>;
        const id = String(record.id ?? "").trim();
        if (!id) return null;
        const label = String(record.label ?? id).trim() || id;
        return { id, label } satisfies UserAgentRoutingModelOption;
      })
      .filter(Boolean) as UserAgentRoutingModelOption[];

    return { providers, models };
  },
};
