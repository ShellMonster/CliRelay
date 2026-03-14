import { apiClient } from "@/lib/http/client";
import {
  normalizeGeminiKeyConfig,
  normalizeOpenAIProvider,
  normalizeProviderKeyConfig,
} from "@/lib/http/transformers";
import type { GeminiKeyConfig, OpenAIProviderConfig, ProviderKeyConfig } from "@/types";

const isRecord = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === "object" && !Array.isArray(value);

const extractArrayPayload = (data: unknown, key: string): unknown[] => {
  if (Array.isArray(data)) return data;
  if (!isRecord(data)) return [];
  const candidate = data[key] ?? data.items ?? data.data ?? data;
  return Array.isArray(candidate) ? candidate : [];
};

const serializeHeaders = (headers?: Record<string, string>) =>
  headers && Object.keys(headers).length ? headers : undefined;

const serializeModelAliases = (models?: ProviderKeyConfig["models"]) =>
  Array.isArray(models)
    ? models
        .map((model) => {
          if (!model?.name) return null;
          const payload: Record<string, unknown> = { name: model.name };
          if (model.alias && model.alias !== model.name) payload.alias = model.alias;
          if (model.priority !== undefined) payload.priority = model.priority;
          if (model.testModel) payload["test-model"] = model.testModel;
          return payload;
        })
        .filter(Boolean)
    : undefined;

const serializeProviderKey = (config: ProviderKeyConfig) => {
  const payload: Record<string, unknown> = { "api-key": config.apiKey };
  if (config.name?.trim()) payload.name = config.name.trim();
  if (config.prefix?.trim()) payload.prefix = config.prefix.trim();
  if (config.baseUrl) payload["base-url"] = config.baseUrl;
  if (config.websockets !== undefined) payload.websockets = config.websockets;
  if (config.proxyUrl) payload["proxy-url"] = config.proxyUrl;
  const headers = serializeHeaders(config.headers);
  if (headers) payload.headers = headers;
  const models = serializeModelAliases(config.models);
  if (models && models.length) payload.models = models;
  if (config.excludedModels?.length) payload["excluded-models"] = config.excludedModels;
  return payload;
};

const serializeVertexKey = (config: ProviderKeyConfig) => {
  const payload: Record<string, unknown> = { "api-key": config.apiKey };
  if (config.name?.trim()) payload.name = config.name.trim();
  if (config.prefix?.trim()) payload.prefix = config.prefix.trim();
  if (config.baseUrl) payload["base-url"] = config.baseUrl;
  if (config.proxyUrl) payload["proxy-url"] = config.proxyUrl;
  const headers = serializeHeaders(config.headers);
  if (headers) payload.headers = headers;
  if (config.models?.length) {
    payload.models = config.models
      .map((model) => {
        const name = model.name?.trim();
        const alias = model.alias?.trim();
        if (!name || !alias) return null;
        return { name, alias };
      })
      .filter(Boolean);
  }
  if (config.excludedModels?.length) payload["excluded-models"] = config.excludedModels;
  return payload;
};

const serializeGeminiKey = (config: GeminiKeyConfig) => {
  const payload: Record<string, unknown> = { "api-key": config.apiKey };
  if (config.name?.trim()) payload.name = config.name.trim();
  if (config.prefix?.trim()) payload.prefix = config.prefix.trim();
  if (config.baseUrl) payload["base-url"] = config.baseUrl;
  if (config.proxyUrl) payload["proxy-url"] = config.proxyUrl;
  const headers = serializeHeaders(config.headers);
  if (headers) payload.headers = headers;
  const models = serializeModelAliases(config.models);
  if (models && models.length) payload.models = models;
  if (config.excludedModels?.length) payload["excluded-models"] = config.excludedModels;
  return payload;
};

const serializeOpenAIProvider = (provider: OpenAIProviderConfig) => {
  const payload: Record<string, unknown> = {
    name: provider.name,
    "base-url": provider.baseUrl,
    "api-key-entries": Array.isArray(provider.apiKeyEntries)
      ? provider.apiKeyEntries.map((entry) => {
          const item: Record<string, unknown> = { "api-key": entry.apiKey };
          if (entry.proxyUrl) item["proxy-url"] = entry.proxyUrl;
          const headers = serializeHeaders(entry.headers);
          if (headers) item.headers = headers;
          return item;
        })
      : [],
  };
  if (provider.prefix?.trim()) payload.prefix = provider.prefix.trim();
  const headers = serializeHeaders(provider.headers);
  if (headers) payload.headers = headers;
  const models = serializeModelAliases(provider.models);
  if (models && models.length) payload.models = models;
  if (provider.excludedModels?.length) payload["excluded-models"] = provider.excludedModels;
  if (provider.priority !== undefined) payload.priority = provider.priority;
  if (provider.testModel) payload["test-model"] = provider.testModel;
  return payload;
};

export const providersApi = {
  async getGeminiKeys(): Promise<GeminiKeyConfig[]> {
    const data = await apiClient.get("/gemini-api-key");
    return extractArrayPayload(data, "gemini-api-key")
      .map((item) => normalizeGeminiKeyConfig(item))
      .filter(Boolean) as GeminiKeyConfig[];
  },

  saveGeminiKeys: (configs: GeminiKeyConfig[]) =>
    apiClient.put(
      "/gemini-api-key",
      configs.map((item) => serializeGeminiKey(item)),
    ),

  updateGeminiKey: (index: number, value: GeminiKeyConfig) =>
    apiClient.patch("/gemini-api-key", { index, value: serializeGeminiKey(value) }),

  deleteGeminiKey: (apiKey: string) =>
    apiClient.delete("/gemini-api-key", undefined, { params: { "api-key": apiKey } }),

  async getCodexConfigs(): Promise<ProviderKeyConfig[]> {
    const data = await apiClient.get("/codex-api-key");
    return extractArrayPayload(data, "codex-api-key")
      .map((item) => normalizeProviderKeyConfig(item))
      .filter(Boolean) as ProviderKeyConfig[];
  },

  saveCodexConfigs: (configs: ProviderKeyConfig[]) =>
    apiClient.put(
      "/codex-api-key",
      configs.map((item) => serializeProviderKey(item)),
    ),

  updateCodexConfig: (index: number, value: ProviderKeyConfig) =>
    apiClient.patch("/codex-api-key", { index, value: serializeProviderKey(value) }),

  deleteCodexConfig: (apiKey: string) =>
    apiClient.delete("/codex-api-key", undefined, { params: { "api-key": apiKey } }),

  async getCodexCompatConfigs(): Promise<ProviderKeyConfig[]> {
    const data = await apiClient.get("/codex-compat-api-key");
    return extractArrayPayload(data, "codex-compat-api-key")
      .map((item) => normalizeProviderKeyConfig(item))
      .filter(Boolean) as ProviderKeyConfig[];
  },

  saveCodexCompatConfigs: (configs: ProviderKeyConfig[]) =>
    apiClient.put(
      "/codex-compat-api-key",
      configs.map((item) => serializeProviderKey(item)),
    ),

  updateCodexCompatConfig: (index: number, value: ProviderKeyConfig) =>
    apiClient.patch("/codex-compat-api-key", { index, value: serializeProviderKey(value) }),

  deleteCodexCompatConfig: (apiKey: string) =>
    apiClient.delete("/codex-compat-api-key", undefined, { params: { "api-key": apiKey } }),

  async getClaudeConfigs(): Promise<ProviderKeyConfig[]> {
    const data = await apiClient.get("/claude-api-key");
    return extractArrayPayload(data, "claude-api-key")
      .map((item) => normalizeProviderKeyConfig(item))
      .filter(Boolean) as ProviderKeyConfig[];
  },

  saveClaudeConfigs: (configs: ProviderKeyConfig[]) =>
    apiClient.put(
      "/claude-api-key",
      configs.map((item) => serializeProviderKey(item)),
    ),

  updateClaudeConfig: (index: number, value: ProviderKeyConfig) =>
    apiClient.patch("/claude-api-key", { index, value: serializeProviderKey(value) }),

  deleteClaudeConfig: (apiKey: string) =>
    apiClient.delete("/claude-api-key", undefined, { params: { "api-key": apiKey } }),

  async getVertexConfigs(): Promise<ProviderKeyConfig[]> {
    const data = await apiClient.get("/vertex-api-key");
    return extractArrayPayload(data, "vertex-api-key")
      .map((item) => normalizeProviderKeyConfig(item))
      .filter(Boolean) as ProviderKeyConfig[];
  },

  saveVertexConfigs: (configs: ProviderKeyConfig[]) =>
    apiClient.put(
      "/vertex-api-key",
      configs.map((item) => serializeVertexKey(item)),
    ),

  updateVertexConfig: (index: number, value: ProviderKeyConfig) =>
    apiClient.patch("/vertex-api-key", { index, value: serializeVertexKey(value) }),

  deleteVertexConfig: (apiKey: string) =>
    apiClient.delete("/vertex-api-key", undefined, { params: { "api-key": apiKey } }),

  async getOpenAIProviders(): Promise<OpenAIProviderConfig[]> {
    const data = await apiClient.get("/openai-compatibility");
    return extractArrayPayload(data, "openai-compatibility")
      .map((item) => normalizeOpenAIProvider(item))
      .filter(Boolean) as OpenAIProviderConfig[];
  },

  saveOpenAIProviders: (providers: OpenAIProviderConfig[]) =>
    apiClient.put(
      "/openai-compatibility",
      providers.map((item) => serializeOpenAIProvider(item)),
    ),

  updateOpenAIProvider: (index: number, value: OpenAIProviderConfig) =>
    apiClient.patch("/openai-compatibility", { index, value: serializeOpenAIProvider(value) }),

  deleteOpenAIProvider: (name: string) =>
    apiClient.delete("/openai-compatibility", undefined, { params: { name } }),

  patchOpenAIProviderByName: (name: string, value: Partial<OpenAIProviderConfig>) => {
    const payload: Record<string, unknown> = {};
    if (value.models !== undefined) {
      payload.models = serializeModelAliases(value.models);
    }
    if (value.excludedModels !== undefined) {
      payload["excluded-models"] = value.excludedModels;
    }
    return apiClient.patch("/openai-compatibility", { name, value: payload });
  },
};
