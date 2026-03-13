import { apiCallApi, getApiCallErrorMessage } from "@/lib/http/apis/api-call";
import { apiClient } from "@/lib/http/client";
import { normalizeModelList } from "@/utils/models";
import type { ModelInfo } from "@/utils/models";

const DEFAULT_CLAUDE_BASE_URL = "https://api.anthropic.com";
const DEFAULT_ANTHROPIC_VERSION = "2023-06-01";
const CLAUDE_MODELS_IN_FLIGHT = new Map<string, Promise<ModelInfo[]>>();

const normalizeApiBase = (baseUrl: string): string => {
  const trimmed = baseUrl.trim();
  if (!trimmed) return "";
  return trimmed.replace(/\/+$/g, "");
};

const buildModelsEndpoint = (baseUrl: string): string => {
  const normalized = normalizeApiBase(baseUrl);
  if (!normalized) return "";
  if (/\/models$/i.test(normalized)) return normalized;
  return `${normalized}/models`;
};

const buildV1ModelsEndpoint = (baseUrl: string): string => {
  const normalized = normalizeApiBase(baseUrl);
  if (!normalized) return "";
  if (/\/v1\/models$/i.test(normalized)) return normalized;
  if (/\/v1$/i.test(normalized)) return `${normalized}/models`;
  return `${normalized}/v1/models`;
};

const buildClaudeModelsEndpoint = (baseUrl: string): string => {
  const normalized = normalizeApiBase(baseUrl);
  const fallback = normalized || DEFAULT_CLAUDE_BASE_URL;
  let trimmed = fallback.replace(/\/+$/g, "");
  trimmed = trimmed.replace(/\/v1\/models$/i, "");
  trimmed = trimmed.replace(/\/v1(?:\/.*)?$/i, "");
  return `${trimmed}/v1/models`;
};

const hasHeader = (headers: Record<string, string>, name: string) => {
  const target = name.toLowerCase();
  return Object.keys(headers).some((key) => key.toLowerCase() === target);
};

const resolveBearerTokenFromAuthorization = (headers: Record<string, string>): string => {
  const entry = Object.entries(headers).find(([key]) => key.toLowerCase() === "authorization");
  if (!entry) return "";
  const value = String(entry[1] ?? "").trim();
  if (!value) return "";
  const match = value.match(/^Bearer\s+(.+)$/i);
  return match?.[1]?.trim() || "";
};

const buildRequestSignature = (url: string, headers: Record<string, string>) => {
  const signature = Object.entries(headers)
    .sort(([a], [b]) => a.toLowerCase().localeCompare(b.toLowerCase()))
    .map(([key, value]) => `${key}:${value}`)
    .join("|");
  return `${url}||${signature}`;
};

export const modelsApi = {
  buildClaudeModelsEndpoint,

  async getStaticModelDefinitions(channel: string) {
    const normalized = String(channel ?? "").trim().toLowerCase();
    if (!normalized) return [] as ModelInfo[];
    const data = await apiClient.get<Record<string, unknown>>(
      `/model-definitions/${encodeURIComponent(normalized)}`,
    );
    return normalizeModelList(data?.models ?? data, { dedupe: true });
  },

  async fetchV1Models(baseUrl: string, apiKey?: string, headers: Record<string, string> = {}) {
    const endpoint = buildV1ModelsEndpoint(baseUrl);
    if (!endpoint) {
      throw new Error("Invalid base url");
    }

    const resolvedHeaders: Record<string, string> = { ...headers };
    if (apiKey) {
      resolvedHeaders.Authorization = `Bearer ${apiKey}`;
    }

    const response = await fetch(endpoint, {
      method: "GET",
      headers: Object.keys(resolvedHeaders).length ? resolvedHeaders : undefined,
    });
    if (!response.ok) {
      const text = await response.text().catch(() => "");
      throw new Error(text.trim() || `请求失败（${response.status}）`);
    }
    const payload = (await response.json().catch(() => null)) as unknown;
    return normalizeModelList(payload, { dedupe: true });
  },

  async fetchModelsViaApiCall(
    baseUrl: string,
    apiKey?: string,
    headers: Record<string, string> = {},
  ) {
    const endpoint = buildModelsEndpoint(baseUrl);
    if (!endpoint) {
      throw new Error("Invalid base url");
    }

    const resolvedHeaders: Record<string, string> = { ...headers };
    const hasAuthHeader =
      typeof resolvedHeaders.Authorization === "string" || hasHeader(resolvedHeaders, "authorization");
    if (apiKey && !hasAuthHeader) {
      resolvedHeaders.Authorization = `Bearer ${apiKey}`;
    }

    const result = await apiCallApi.request({
      method: "GET",
      url: endpoint,
      header: Object.keys(resolvedHeaders).length ? resolvedHeaders : undefined,
    });

    if (result.statusCode < 200 || result.statusCode >= 300) {
      throw new Error(getApiCallErrorMessage(result));
    }

    return normalizeModelList(result.body ?? result.bodyText, { dedupe: true });
  },

  async fetchClaudeModelsViaApiCall(
    baseUrl: string,
    apiKey?: string,
    headers: Record<string, string> = {},
  ) {
    const endpoint = buildClaudeModelsEndpoint(baseUrl);
    if (!endpoint) {
      throw new Error("Invalid base url");
    }

    const resolvedHeaders: Record<string, string> = { ...headers };
    let resolvedApiKey = String(apiKey ?? "").trim();
    if (!resolvedApiKey && !hasHeader(resolvedHeaders, "x-api-key")) {
      resolvedApiKey = resolveBearerTokenFromAuthorization(resolvedHeaders);
    }

    if (resolvedApiKey && !hasHeader(resolvedHeaders, "x-api-key")) {
      resolvedHeaders["x-api-key"] = resolvedApiKey;
    }
    if (!hasHeader(resolvedHeaders, "anthropic-version")) {
      resolvedHeaders["anthropic-version"] = DEFAULT_ANTHROPIC_VERSION;
    }

    const signature = buildRequestSignature(endpoint, resolvedHeaders);
    const existing = CLAUDE_MODELS_IN_FLIGHT.get(signature);
    if (existing) return existing;

    const request = (async () => {
      const result = await apiCallApi.request({
        method: "GET",
        url: endpoint,
        header: Object.keys(resolvedHeaders).length ? resolvedHeaders : undefined,
      });

      if (result.statusCode < 200 || result.statusCode >= 300) {
        throw new Error(getApiCallErrorMessage(result));
      }

      return normalizeModelList(result.body ?? result.bodyText, { dedupe: true });
    })();

    CLAUDE_MODELS_IN_FLIGHT.set(signature, request);
    try {
      return await request;
    } finally {
      CLAUDE_MODELS_IN_FLIGHT.delete(signature);
    }
  },
};
