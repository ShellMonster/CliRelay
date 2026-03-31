/**
 * AI 提供商相关类型
 * 基于原项目 src/modules/ai-providers.js
 */

export interface ModelAlias {
  name: string;
  alias?: string;
  priority?: number;
  testModel?: string;
}

export interface ApiKeyEntry {
  apiKey: string;
  proxyUrl?: string;
  headers?: Record<string, string>;
}

export interface GeminiKeyConfig {
  apiKey: string;
  name?: string;
  prefix?: string;
  baseUrl?: string;
  proxyUrl?: string;
  participateInDefaultRouting?: boolean;
  autoSyncModels?: boolean;
  models?: ModelAlias[];
  headers?: Record<string, string>;
  excludedModels?: string[];
}

export interface ProviderKeyConfig {
  apiKey: string;
  name?: string;
  prefix?: string;
  baseUrl?: string;
  websockets?: boolean;
  proxyUrl?: string;
  participateInDefaultRouting?: boolean;
  autoSyncModels?: boolean;
  headers?: Record<string, string>;
  models?: ModelAlias[];
  excludedModels?: string[];
}

export interface OpenAIProviderConfig {
  name: string;
  prefix?: string;
  baseUrl: string;
  participateInDefaultRouting?: boolean;
  autoSyncModels?: boolean;
  apiKeyEntries: ApiKeyEntry[];
  headers?: Record<string, string>;
  models?: ModelAlias[];
  excludedModels?: string[];
  priority?: number;
  testModel?: string;
  [key: string]: unknown;
}
