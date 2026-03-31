import { apiCallApi, getApiCallErrorMessage } from "@/lib/http/apis/api-call";
import { apiClient } from "@/lib/http/client";
import { normalizeModelList } from "@/utils/models";
import type { ModelInfo } from "@/utils/models";

const DEFAULT_CLAUDE_BASE_URL = "https://api.anthropic.com";
const DEFAULT_ANTHROPIC_VERSION = "2023-06-01";
const MODELS_IN_FLIGHT = new Map<string, Promise<ModelInfo[]>>();

export type ModelDiscoveryProvider =
  | "openai"
  | "claude"
  | "codex"
  | "codex-compat"
  | "copilot-compat"
  | "gemini";

const normalizeApiBase = (baseUrl: string): string => {
  let trimmed = baseUrl.trim();
  if (!trimmed) return "";
  trimmed = trimmed.replace(/\/?v0\/management\/?$/i, "");
  trimmed = trimmed.replace(/\/+$/g, "");
  if (!/^https?:\/\//i.test(trimmed)) {
    trimmed = `http://${trimmed}`;
  }
  return trimmed;
};

const buildDiscoveryBase = (baseUrl: string, fallbackBaseUrl = ""): string => {
  const normalized = normalizeApiBase(baseUrl) || normalizeApiBase(fallbackBaseUrl);
  if (!normalized) return "";
  return normalized.replace(/\/v1\/models$/i, "").replace(/\/models$/i, "");
};

const buildRootModelsEndpoint = (baseUrl: string, fallbackBaseUrl = ""): string => {
  const normalized = buildDiscoveryBase(baseUrl, fallbackBaseUrl);
  if (!normalized) return "";
  return `${normalized.replace(/\/v1$/i, "")}/models`;
};

const buildV1ModelsEndpoint = (baseUrl: string, fallbackBaseUrl = ""): string => {
  const normalized = buildDiscoveryBase(baseUrl, fallbackBaseUrl);
  if (!normalized) return "";
  if (/\/v1$/i.test(normalized)) {
    return `${normalized}/models`;
  }
  return `${normalized}/v1/models`;
};

const hasHeader = (headers: Record<string, string>, name: string) => {
  const target = name.toLowerCase();
  return Object.keys(headers).some((key) => key.toLowerCase() === target);
};

const getHeaderValue = (headers: Record<string, string>, name: string): string => {
  const target = name.toLowerCase();
  const entry = Object.entries(headers).find(([key]) => key.toLowerCase() === target);
  return String(entry?.[1] ?? "").trim();
};

const resolveBearerTokenFromAuthorization = (headers: Record<string, string>): string => {
  const value = getHeaderValue(headers, "authorization");
  if (!value) return "";
  const match = value.match(/^Bearer\s+(.+)$/i);
  return match?.[1]?.trim() || "";
};

const resolveApiKeyCandidate = (
  apiKey: string | undefined,
  headers: Record<string, string>,
): string => {
  const fieldValue = String(apiKey ?? "").trim();
  if (fieldValue) return fieldValue;

  const bearerValue = resolveBearerTokenFromAuthorization(headers);
  if (bearerValue) return bearerValue;

  return getHeaderValue(headers, "x-api-key");
};

const buildRequestSignature = (url: string, headers: Record<string, string>) => {
  const signature = Object.entries(headers)
    .sort(([a], [b]) => a.toLowerCase().localeCompare(b.toLowerCase()))
    .map(([key, value]) => `${key}:${value}`)
    .join("|");
  return `${url}||${signature}`;
};

const buildDiscoverySignature = (
  provider: ModelDiscoveryProvider,
  attempts: Array<{ url: string; headers: Record<string, string> }>,
) =>
  `${provider}||${attempts.map((attempt) => buildRequestSignature(attempt.url, attempt.headers)).join(";;")}`;

const buildOpenAIAttemptHeaders = (
  apiKey: string | undefined,
  headers: Record<string, string>,
): Record<string, string> => {
  const resolvedHeaders: Record<string, string> = { ...headers };
  const hasAuthHeader = hasHeader(resolvedHeaders, "authorization");
  const resolvedApiKey = resolveApiKeyCandidate(apiKey, resolvedHeaders);
  if (resolvedApiKey && !hasAuthHeader) {
    resolvedHeaders.Authorization = `Bearer ${resolvedApiKey}`;
  }
  return resolvedHeaders;
};

const buildAnthropicAttemptHeaders = (
  apiKey: string | undefined,
  headers: Record<string, string>,
): Record<string, string> => {
  const resolvedHeaders: Record<string, string> = { ...headers };
  const resolvedApiKey = resolveApiKeyCandidate(apiKey, resolvedHeaders);
  if (resolvedApiKey && !hasHeader(resolvedHeaders, "x-api-key")) {
    resolvedHeaders["x-api-key"] = resolvedApiKey;
  }
  if (!hasHeader(resolvedHeaders, "anthropic-version")) {
    resolvedHeaders["anthropic-version"] = DEFAULT_ANTHROPIC_VERSION;
  }
  return resolvedHeaders;
};

const dedupeEndpoints = (endpoints: string[]) => {
  const seen = new Set<string>();
  return endpoints.filter((endpoint) => {
    if (!endpoint) return false;
    if (seen.has(endpoint)) return false;
    seen.add(endpoint);
    return true;
  });
};

const buildModelDiscoveryEndpoints = (
  provider: ModelDiscoveryProvider,
  baseUrl: string,
): string[] => {
  if (provider === "claude") {
    return dedupeEndpoints([
      buildV1ModelsEndpoint(baseUrl, DEFAULT_CLAUDE_BASE_URL),
      buildRootModelsEndpoint(baseUrl, DEFAULT_CLAUDE_BASE_URL),
    ]);
  }

  return dedupeEndpoints([buildRootModelsEndpoint(baseUrl), buildV1ModelsEndpoint(baseUrl)]);
};

const buildDiscoveryAttempts = (
  provider: ModelDiscoveryProvider,
  baseUrl: string,
  apiKey: string | undefined,
  headers: Record<string, string>,
) => {
  const endpoints = buildModelDiscoveryEndpoints(provider, baseUrl);
  if (provider === "claude") {
    return endpoints.map((url, index) => ({
      url,
      headers:
        index === 0
          ? buildAnthropicAttemptHeaders(apiKey, headers)
          : buildOpenAIAttemptHeaders(apiKey, headers),
    }));
  }

  return endpoints.map((url) => ({
    url,
    headers: buildOpenAIAttemptHeaders(apiKey, headers),
  }));
};

const getUnknownErrorMessage = (err: unknown) => {
  if (err instanceof Error) return err.message;
  if (typeof err === "string") return err;
  return "请求失败";
};

const fetchProviderModelsViaApiCall = async (
  provider: ModelDiscoveryProvider,
  baseUrl: string,
  apiKey?: string,
  headers: Record<string, string> = {},
) => {
  const attempts = buildDiscoveryAttempts(provider, baseUrl, apiKey, headers);
  if (attempts.length === 0) {
    throw new Error("Invalid base url");
  }

  const signature = buildDiscoverySignature(provider, attempts);
  const existing = MODELS_IN_FLIGHT.get(signature);
  if (existing) return existing;

  const request = (async () => {
    const errors: string[] = [];

    for (const attempt of attempts) {
      try {
        const result = await apiCallApi.request({
          method: "GET",
          url: attempt.url,
          header: Object.keys(attempt.headers).length ? attempt.headers : undefined,
        });

        if (result.statusCode < 200 || result.statusCode >= 300) {
          throw new Error(getApiCallErrorMessage(result));
        }

        return normalizeModelList(result.body ?? result.bodyText, { dedupe: true });
      } catch (err: unknown) {
        errors.push(`${attempt.url}: ${getUnknownErrorMessage(err)}`);
      }
    }

    if (errors.length === 1) {
      throw new Error(errors[0]);
    }
    throw new Error(errors.join("；"));
  })();

  MODELS_IN_FLIGHT.set(signature, request);
  try {
    return await request;
  } finally {
    MODELS_IN_FLIGHT.delete(signature);
  }
};

export const modelsApi = {
  buildClaudeModelsEndpoint(baseUrl: string) {
    return buildModelDiscoveryEndpoints("claude", baseUrl)[0] ?? "";
  },

  buildModelDiscoveryEndpoints,

  fetchProviderModelsViaApiCall,

  async getStaticModelDefinitions(channel: string) {
    const normalized = String(channel ?? "")
      .trim()
      .toLowerCase();
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
    return fetchProviderModelsViaApiCall("openai", baseUrl, apiKey, headers);
  },

  async fetchClaudeModelsViaApiCall(
    baseUrl: string,
    apiKey?: string,
    headers: Record<string, string> = {},
  ) {
    return fetchProviderModelsViaApiCall("claude", baseUrl, apiKey, headers);
  },
};
